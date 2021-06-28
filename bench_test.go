package main

import (
	"fmt"
	"os"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

func BenchmarkRun(b *testing.B) {
	os.Stdout = nil

	p := gol.Params{

		Turns:       100,
		ImageWidth:  512,
		ImageHeight: 512,
	}

	//keyPresses := make(chan rune, 10)

	for i := 0; i < b.N; i++ {

		for threads := 1; threads <= 16; threads++ {
			p.Threads = threads

			Name := fmt.Sprintf("%dx%dx%d-%d", p.ImageWidth, p.ImageHeight, p.Turns, p.Threads)
			b.Run(fmt.Sprint(Name), func(b *testing.B) {
				events := make(chan gol.Event, 1000)
				gol.Run(p, events, nil)
				for range events {
				}
			})
		}

	}

}
