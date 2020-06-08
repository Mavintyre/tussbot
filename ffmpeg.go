package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dougty/tussbot/ogg" // github.com/jonas747/ogg
)

var userAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:77.0) Gecko/20100101 Firefox/77.0"
var referer = "https://www.youtube.com/"

// FFMPEGSession of encoder & streamer
type FFMPEGSession struct {
	sync.Mutex
	encoding    bool
	streaming   bool
	ffmpeg      *os.Process
	frameBuffer chan []byte
	done        chan error
	killDecoder chan int
	paused      bool
	framesSent  int
	voiceCh     *discordgo.VoiceConnection
}

var frameDuration = 20 // 20, 40, or 60 ms

// Start an ffmpeg session and begin streaming
//	`done` channel signals io.EOF for natural end of stream as well as legitimate errors
func (s *FFMPEGSession) Start(url string, seek int, volume float64, vc *discordgo.VoiceConnection, bitrate int, done chan error) {
	s.done = done
	s.voiceCh = vc

	s.Lock()
	if s.encoding || s.streaming {
		done <- errors.New("invalid attempt to restart ffmpeg session")
		s.Unlock()
		return
	}

	s.encoding = true
	s.paused = false
	s.framesSent = 0

	// TO DO: better to kill on pause in music.go instead of pause here? how does buffer react

	args := []string{
		"-ss", strconv.Itoa(seek),
		"-i", url,

		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "4",

		"-user_agent", userAgent,
		"-referer", referer,

		"-vn",
		"-map", "0:a",

		"-acodec", "libopus",
		"-f", "ogg",

		"-analyzeduration", "0",
		"-probesize", "1000000", // 1mb - min 32 default 5000000
		"-avioflags", "direct",
		"-fflags", "+fastseek+nobuffer+flush_packets+discardcorrupt",
		"-flush_packets", "1",

		"-vbr", "on",
		"-compression_level", "10", // 0-10, higher = better but slower
		"-application", "audio", // voip = speech, audio, lowdelay
		"-frame_duration", strconv.Itoa(frameDuration),
		// pcm frame length = 960 * channels * ( framedur / 20 )
		"-packet_loss", "10", // expected %

		"-ar", "48000",
		"-ac", "2",

		"-b:a", strconv.Itoa(bitrate),
		"-af", fmt.Sprintf("loudnorm,volume=%.2f", volume),

		"-loglevel", "16",
		"pipe:1",
	}

	cmd := exec.Command("ffmpeg", args...)
	s.killDecoder = make(chan int, 1)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.Unlock()
		done <- fmt.Errorf("error starting stdout pipe: %w", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.Unlock()
		done <- fmt.Errorf("error starting stderr pipe: %w", err)
		return
	}

	// TO DO: change buffer length?
	s.frameBuffer = make(chan []byte, frameDuration*5)
	defer close(s.frameBuffer)

	err = cmd.Start()
	if err != nil {
		s.Unlock()
		done <- fmt.Errorf("error starting ffmpeg process: %w", err)
		return
	}

	s.ffmpeg = cmd.Process
	s.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go s.StartStream()
	go s.readStdout(stdout, &wg)
	go s.readStderr(stderr, &wg)

	wg.Wait()

	err = cmd.Wait()
	if err != nil {
		if err.Error() != "signal: killed" {
			done <- fmt.Errorf("ffmpeg error: %w", err)
		}
	}

	s.Lock()
	s.encoding = false
	s.Unlock()
}

func (s *FFMPEGSession) readStderr(stderr io.ReadCloser, wg *sync.WaitGroup) {
	defer wg.Done()

	bufReader := bufio.NewReader(stderr)
	var outBuf bytes.Buffer
	for {
		r, _, err := bufReader.ReadRune()
		if err != nil {
			if err != io.EOF {
				s.done <- fmt.Errorf("ffmpeg stderr error: %w", err)
			}
			break
		}

		if r == '\n' {
			// TO DO: save to string, send error to owner on encoding completion
			fmt.Println("[ffmpeg] ", outBuf.String())
			outBuf.Reset()
		} else {
			outBuf.WriteRune(r)
		}
	}
	stderr.Close()
}

func (s *FFMPEGSession) readStdout(stdout io.ReadCloser, wg *sync.WaitGroup) {
	defer wg.Done()

	decoder := ogg.NewPacketDecoder(ogg.NewDecoder(stdout))

	skip := 2
	for {
		select {
		// channel required to kill this goroutine as decoder.Decode will block forever
		// and the chances of us intercepting that are slim to none
		case <-s.killDecoder:
			stdout.Close()
			return
		default:
			packet, _, err := decoder.Decode()
			if skip > 0 {
				skip--
				continue
			}
			if err != nil {
				if err != io.EOF && err != io.ErrUnexpectedEOF {
					s.done <- fmt.Errorf("ffmpeg stdout error: %w", err)
				}
				s.frameBuffer <- nil
				break
			}

			s.frameBuffer <- packet
		}
	}
}

// GetFrame returns a single frame of DCA encoded Opus
func (s *FFMPEGSession) GetFrame() (frame []byte, err error) {
	f := <-s.frameBuffer
	if f == nil {
		return nil, io.EOF
	}

	return f, nil
}

// StartStream to discordgo voice connection
func (s *FFMPEGSession) StartStream() {
	s.Lock()

	if s.streaming || s.paused {
		s.Unlock()
		return
	}

	s.streaming = true
	s.Unlock()

	for {
		s.Lock()
		if s.paused {
			s.Unlock()
			break
		}
		s.Unlock()

		frame, err := s.GetFrame()
		if err != nil {
			s.done <- fmt.Errorf("error getting opus frame: %w", err)
			break
		}

		// timeout after 100ms
		// TO DO: is this adequate? too big? too small?
		timeout := time.After(time.Second)

		select {
		case <-timeout:
			s.done <- errors.New("voice connection timed out")
			break
		case s.voiceCh.OpusSend <- frame:
			// packet has been sent
		}

		s.Lock()
		s.framesSent++
		s.Unlock()
	}

	s.Lock()
	s.streaming = false
	s.Unlock()
}

// CurrentTime returns current playback position
func (s *FFMPEGSession) CurrentTime() time.Duration {
	s.Lock()
	defer s.Unlock()
	return time.Duration(s.framesSent*frameDuration) * time.Millisecond
}

// SetPaused pauses or resumes streaming
func (s *FFMPEGSession) SetPaused(p bool) {
	s.Lock()
	defer s.Unlock()

	// paused == true will break the stream loop
	s.paused = p
	if p == false {
		go s.StartStream()
	}
}

// Paused returns whether the stream is currently paused
func (s *FFMPEGSession) Paused() bool {
	s.Lock()
	defer s.Unlock()

	return s.paused
}

// Cleanup kill process and clean up remaining unstreamed frames
func (s *FFMPEGSession) Cleanup() {
	// stop streamer
	if s.streaming {
		s.SetPaused(true) // stop sending packets
		// wait until stream has closed
		for {
			s.Lock()
			if !s.streaming {
				s.Unlock()
				break
			}
			s.Unlock()
		}
	}

	// kill process
	s.Lock()
	if s.ffmpeg != nil {
		s.ffmpeg.Kill()
	}
	s.Unlock()

	// kill decoder goroutine
	if s.encoding {
		s.killDecoder <- 1
	}

	// empty remaining frames in buffer
	for len(s.frameBuffer) > 0 {
		<-s.frameBuffer
	}

	// wait until encoder has closed
	for {
		s.Lock()
		if !s.encoding {
			s.Unlock()
			break
		}
		s.Unlock()
	}
}

// Stop everything and clean up
func (s *FFMPEGSession) Stop() {
	s.Cleanup()
	s.done <- fmt.Errorf("stopped on request %w", io.EOF)
}
