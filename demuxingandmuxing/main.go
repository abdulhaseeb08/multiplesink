package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tinyzimmer/go-glib/glib"
	"github.com/tinyzimmer/go-gst/examples"
	"github.com/tinyzimmer/go-gst/gst"
)

var srcFile string

func buildPipeline() (*gst.Pipeline, error) {

	//initialize gstreamer
	gst.Init(nil)

	//create a new pipeline
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return nil, err
	}

	//create a new filesource element to read our file from the current directory
	src, err := gst.NewElement("filesrc")
	if err != nil {
		return nil, err
	}

	decodebin, err := gst.NewElement("decodebin")
	if err != nil {
		return nil, err
	}

	src.Set("location", srcFile)

	pipeline.AddMany(src, decodebin)
	src.Link(decodebin)

	//building the webmmux
	mux, err := gst.NewElement("webmmux")
	if err != nil {
		return nil, err
	}

	//request an audio sink pad
	muxAudio := mux.GetRequestPad("audio_%u")
	//request a video sink pad
	muxVideo := mux.GetRequestPad("video_%u")

	filesink, err := gst.NewElement("filesink")
	if err != nil {
		return nil, err
	}

	pipeline.AddMany(mux, filesink)
	mux.SyncStateWithParent()
	filesink.SyncStateWithParent()
	mux.Link(filesink)

	filesink.Set("location", "testtyy.webm")

	//our demuxer at first doesnt have source pads, they are created as the
	//demuxer interprets data and creates source pads on the fly, so we
	//need to link them accordingly. For that we add the pad-added signal
	decodebin.Connect("pad-added", func(self *gst.Element, srcPad *gst.Pad) {

		// We detect audio and video streams here
		var audioStream, videoStream bool
		caps := srcPad.GetCurrentCaps()
		for i := 0; i < caps.GetSize(); i++ {
			st := caps.GetStructureAt(i)
			if strings.HasPrefix(st.Name(), "audio/") {
				audioStream = true
			}
			if strings.HasPrefix(st.Name(), "video/") {
				videoStream = true
			}
		}

		fmt.Printf("New pad added, Audio=%v, Video=%v\n", audioStream, videoStream)

		if !audioStream && !videoStream {
			err := errors.New("could not detect media stream type")
			msg := gst.NewErrorMessage(self, gst.NewGError(1, err), fmt.Sprintf("Received caps: %s", caps.String()), nil)
			pipeline.GetPipelineBus().Post(msg)
			return
		}

		if audioStream {
			//we create the following pipeline if audioStream is found
			elements, err := gst.NewElementMany("queue", "audioconvert", "vorbisenc", "queue")
			if err != nil {
				msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for audio pipeline", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}
			pipeline.AddMany(elements...)
			gst.ElementLinkMany(elements...)

			//making sure the elements have the same state as the pipeline
			for _, e := range elements {
				e.SyncStateWithParent()
			}

			elements[3].GetStaticPad("src").Link(muxAudio)

			// The queue was the first element returned above
			queue := elements[0]
			// Get the queue element's sink pad and link the decodebin's newly created
			// src pad for the audio stream to it.
			sinkPad := queue.GetStaticPad("sink")
			srcPad.Link(sinkPad)

		}
		if videoStream {
			//we create the following pipeline if audioStream is found
			elements, err := gst.NewElementMany("queue", "videoconvert", "vp8enc", "queue")
			if err != nil {
				msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for video pipeline", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}
			pipeline.AddMany(elements...)
			gst.ElementLinkMany(elements...)

			for _, e := range elements {
				e.SyncStateWithParent()
			}

			elements[3].GetStaticPad("src").Link(muxVideo)

			queue := elements[0]
			// Get the queue element's sink pad and link the decodebin's newly created
			// src pad for the video stream to it.
			sinkPad := queue.GetStaticPad("sink")
			srcPad.Link(sinkPad)

		}

	})
	return pipeline, nil

}

func runPipeline(loop *glib.MainLoop, pipeline *gst.Pipeline) error {
	// Start the pipeline
	pipeline.SetState(gst.StatePlaying)

	// Add a message watch to the bus to quit on any error
	pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		var err error

		// If the stream has ended or any element posts an error to the
		// bus, populate error.
		switch msg.Type() {
		case gst.MessageEOS:
			err = errors.New("end-of-stream")
		case gst.MessageError:
			// The parsed error implements the error interface, but also
			// contains additional debug information.
			gerr := msg.ParseError()
			fmt.Println("go-gst-debug:", gerr.DebugString())
			err = gerr
		}

		// If either condition triggered an error, log and quit
		if err != nil {
			fmt.Println("ERROR:", err.Error())
			loop.Quit()
			return false
		}

		return true
	})

	// Block on the main loop
	return loop.RunError()
}

func main() {
	flag.StringVar(&srcFile, "f", "", "The file to decode")
	flag.Parse()
	if srcFile == "" {
		flag.Usage()
		os.Exit(1)
	}
	examples.RunLoop(func(loop *glib.MainLoop) error {
		pipeline, err := buildPipeline()
		if err != nil {
			return err
		}
		return runPipeline(loop, pipeline)
	})
}
