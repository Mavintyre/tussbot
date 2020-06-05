package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
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

type frame struct {
	data     []byte
	metaData bool
}

// Session of ffmpeg encoder & streamer
type Session struct {
	sync.Mutex
	running      bool
	streaming    bool
	started      time.Time
	process      *os.Process
	frameChannel chan *frame
	done         chan error
	paused       bool
	volume       float64
	framesSent   int
	streamvc     *discordgo.VoiceConnection
}

var frameDuration = 20 // 20, 40, or 60 ms

// TO DO: usage
//	done := make(chan error)
//	session := Start(url, vc, done)
//	ticker := time.NewTicker(time.Second*10)
//	for loop, select
//	case err:= <-done
//	unwrap err
//	if != nil and != io.EOF
//		send error
//	case <-ticker.C
//	get session.time() & update embed

// Start an ffmpeg session and begin streaming
//	`done` channel can signal io.EOF for natural end of stream or a legitimate error
func (s *Session) Start(url string, vc *discordgo.VoiceConnection, done chan error) {
	defer s.Unlock()

	s.Lock()
	if s.running {
		s.Stop()
	}

	defer close(s.frameChannel)
	s.running = true

	args := []string{
		// "-ss", seek,
		"-i", url,

		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "4",

		//"-user_agent", user_agent,
		//"-referer", referer,

		"-vn",
		"-map", "0:a", // audio only

		"-acodec", "libopus",
		"-f", "opus", // ogg

		"-analyzeduration", "0",
		"-probesize", "1000000", // 1mb - min 32 default 5000000
		"-avioflags", "direct",
		"-fflags", "+fastseek+nobuffer+flush_packets+discardcorrupt",
		"-flush_packets", "1",

		//"-vbr", "off" // on
		"-compression_level", "10", // 0-10, higher = better but slower
		"-application", "audio", // voip = speech, audio, lowdelay
		"-frame_duration", strconv.Itoa(frameDuration),
		// pcm frame length = 960 * channels * ( framedur / 20 )
		"-packet_loss", "10", // expected %
		"-threads", "0",

		"-ar", "48000",
		"-ac", "2",

		"-b:a", "64000",
		"-af", fmt.Sprintf("loudnorm,volume=%.2f", s.volume),

		"-loglevel", "16",
		"pipe:1",
	}

	cmd := exec.Command("ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		done <- fmt.Errorf("error starting stdout pipe: %w", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		done <- fmt.Errorf("error starting stderr pipe: %w", err)
		return
	}

	err = cmd.Start()
	if err != nil {
		done <- fmt.Errorf("error starting ffmpeg process: %w", err)
		return
	}

	s.started = time.Now()
	s.process = cmd.Process
	s.streamvc = vc
	s.Unlock() // unlock early

	go s.StartStream()

	var wg sync.WaitGroup
	wg.Add(2)
	go s.readStderr(stderr, &wg)
	go s.readStdout(stdout, &wg)
	wg.Wait()

	err = cmd.Wait()
	if err != nil {
		if err.Error() != "signal: killed" {
			done <- fmt.Errorf("ffmpeg error: %w", err)
		}
	}
	s.running = false
}

func (s *Session) readStderr(stderr io.ReadCloser, wg *sync.WaitGroup) {
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
			fmt.Println("[ffmpeg] ", outBuf.String())
			outBuf.Reset()
		} else {
			outBuf.WriteRune(r)
		}
	}
}

func (s *Session) readStdout(stdout io.ReadCloser, wg *sync.WaitGroup) {
	defer wg.Done()

	decoder := ogg.NewPacketDecoder(ogg.NewDecoder(stdout))

	skip := 2
	for {
		packet, _, err := decoder.Decode()
		if skip > 0 {
			skip--
			continue
		}
		if err != nil {
			if err != io.EOF {
				s.done <- fmt.Errorf("ffmpeg stdout error: %w", err)
			}
			break
		}

		err = s.writeDCAFrame(packet)
		if err != nil {
			s.done <- fmt.Errorf("error writing dca frame: %w", err)
			break
		}
	}
}

func (s *Session) writeDCAFrame(opusFrame []byte) error {
	var dcaBuf bytes.Buffer

	err := binary.Write(&dcaBuf, binary.LittleEndian, uint16(len(opusFrame)))
	if err != nil {
		return err
	}

	_, err = dcaBuf.Write(opusFrame)
	if err != nil {
		return err
	}

	s.frameChannel <- &frame{dcaBuf.Bytes(), false}
	return nil
}

// Frame returns a single frame of DCA encoded Opus
func (s *Session) Frame() (frame []byte, err error) {
	f := <-s.frameChannel
	if f == nil {
		return nil, io.EOF
	}

	if len(f.data) < 2 {
		return nil, errors.New("bad frame")
	}

	return f.data[2:], nil
}

// StartStream to discordgo voice connection
func (s *Session) StartStream() {
	s.Lock()
	defer s.Unlock()

	if s.streaming {
		return
	}

	for {
		if s.paused || !s.streaming {
			return
		}
		s.Unlock() // unlock early

		s.streaming = true
		frame, err := s.Frame()
		if err != nil {
			s.done <- fmt.Errorf("error getting dca frame: %w", err)
			break
		}

		// timeout after 100ms (maybe this needs to be changed?)
		timeout := time.After(time.Second)

		// try to send on the voice channel before the timeout
		select {
		case <-timeout:
			s.done <- errors.New("voice connection timed out")
			break
		case s.streamvc.OpusSend <- frame:
		}

		s.Lock()
		s.framesSent++
		s.Unlock()
	}

	s.Lock()
	s.streaming = false
	s.Unlock()
}

// Time returns current playback position
func (s *Session) Time() time.Duration {
	s.Lock()
	defer s.Unlock()
	return time.Duration(s.framesSent) * time.Duration(frameDuration) * time.Millisecond
}

// SetPaused state and stop or restart stream
func (s *Session) SetPaused(p bool) {
	s.Lock()
	defer s.Unlock()

	s.paused = p
	if p {
		s.StopStream()
	} else {
		s.StartStream()
	}
}

// StopEncoder kill process and clean up remaining unstreamed frames
func (s *Session) StopEncoder() {
	s.Lock()
	defer s.Unlock()

	if s.process != nil {
		s.process.Kill()
	}
	s.running = false

	for range s.frameChannel {
		// empty remaining frames
	}
}

// StopStream sets streaming=false, stopping the streaming loop goroutine
func (s *Session) StopStream() {
	s.Lock()
	defer s.Unlock()
	s.streaming = false
}

// Stop stream, encoder, ffmpeg process, and clean up
func (s *Session) Stop() {
	s.StopStream()
	s.StopEncoder()
}
