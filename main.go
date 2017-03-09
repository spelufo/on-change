package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

func main() {
	debounce := 300 * time.Millisecond
	eventMask := fsnotify.Create | fsnotify.Write | fsnotify.Chmod | fsnotify.Rename
	globs := []string{}

	i := 1
	for i < len(os.Args)-1 {
		if os.Args[i] == "-d" {
			var err error
			debounce, err = time.ParseDuration(os.Args[i+1])
			if err != nil {
				usageError()
			}
			i += 2
			continue
		}
		if os.Args[i] == "-m" {
			eventMask = 0
			for _, c := range os.Args[i+1] {
				switch c {
				case 'c':
					eventMask |= fsnotify.Create
				case 'w':
					eventMask |= fsnotify.Write
				case 'r':
					eventMask |= fsnotify.Remove
				case 'm':
					eventMask |= fsnotify.Rename
				case 'a':
					eventMask |= fsnotify.Chmod
				}
			}
			i += 2
			continue
		}
		if os.Args[i] == "-g" {
			globs = append(globs, os.Args[i+1])
			i += 2
			continue
		}

		break
	}

	args := os.Args[i:]
	if len(args) < 2 {
		usageError()
	}

	files := args[:len(args)-1]
	command := args[len(args)-1]

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer watcher.Close()

	failed := 0
	for _, f := range files {
		err := watcher.Add(f)
		if err != nil {
			os.Stderr.WriteString("can't watch "+f+": "+err.Error()+"\n")
			failed++
		}
	}
	if failed == len(files) {
		os.Exit(1)
	}

	events := map[string]fsnotify.Op{}
	timer := time.NewTimer(debounce)
	timer.Stop()

	for {
		select {
		case <-timer.C:
			args := make([]string, 2, len(events) + 2)
			args[0] = "-c"
			args[1] = command
			for ev := range events {
				args = append(args, ev)
			}
			cmd := exec.Command(shell, args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()

			events = map[string]fsnotify.Op{}

		case ev := <-watcher.Events:
			match := false
			for _, glob := range globs {
				if matched, _ := filepath.Match(glob, filepath.Base(ev.Name)); matched {
					match = true
				}
			}
			if (match || len(globs) == 0) && ev.Op&eventMask == ev.Op {
				events[ev.Name] = ev.Op

				// docs say not to call reset concurrently with <-timer.C
				// is this ok? haven't had problems yet
				timer.Reset(debounce)
			}

		case err := <-watcher.Errors:
			os.Stderr.WriteString(err.Error())
		}
	}
}

func usageError() {
	os.Stderr.WriteString(`usage: on-change [-d debounce] [-e eventmask] [-g glob] FILES... CMD
	-d  debounce time. (default: 300ms)
	-e  event mask. (default: cwma)
	    include these characters to listen for these events:
		c create
		w write
		r remove
		m rename (move)
		a chmod (access)
	-g  trigger events only when the file basename matches one of the given globs.`)
	os.Exit(2)
}