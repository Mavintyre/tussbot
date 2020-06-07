package main

func init() {
	RegisterCommand(Command{
		aliases: []string{"play", "p"},
		help:    "play a song from url",
		callback: func(ca CommandArgs) {
			// parse url
			// get streamurl
			// join channel
			// begin and save ref to ffmpeg session
		},
	})
}
