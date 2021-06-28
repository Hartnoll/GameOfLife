package gol

import (
	"fmt"
	"time"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events        chan<- Event
	ioCommand     chan<- ioCommand
	ioIdle        <-chan bool
	ioFilename    chan string
	ioImageInput  chan byte
	ioImageOutput chan byte
	keyPresses    <-chan rune
}

const alive = 255
const dead = 0

func mod(x, m int) int {
	return (x + m) % m
}

//CalculateNeighbours takes in a pixel and returns an integer value for the amount of alive neighbours.
func calculateNeighbours(p Params, x, y int, world [][]byte) int {
	neighbours := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i != 0 || j != 0 {
				if world[mod(y+i, p.ImageHeight)][mod(x+j, p.ImageWidth)] == alive {
					neighbours++
				}
			}
		}
	}
	return neighbours
}

//CalculateNextState returns the 2d matrix corresponding to the next state of the game.
func calculateNextState(p Params, StartY, EndY int, world [][]byte, c distributorChannels, t int) [][]byte {
	newWorld := make([][]byte, (EndY - StartY))
	for i := range newWorld {
		newWorld[i] = make([]byte, p.ImageWidth)
	}
	i := 0
	for y := StartY; y < EndY; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			neighbours := calculateNeighbours(p, x, y, world)
			if world[y][x] == alive {
				if neighbours == 2 || neighbours == 3 {
					newWorld[i][x] = alive
				} else {
					newWorld[i][x] = dead
					flippedCell := util.Cell{
						X: x,
						Y: y,
					}
					flipped := CellFlipped{
						CompletedTurns: t,
						Cell:           flippedCell,
					}
					c.events <- flipped
				}
			} else {
				if neighbours == 3 {
					newWorld[i][x] = alive
					flippedCell := util.Cell{
						X: x,
						Y: y,
					}
					flipped := CellFlipped{
						CompletedTurns: t,
						Cell:           flippedCell,
					}
					c.events <- flipped

				} else {
					newWorld[i][x] = dead
				}
			}
		}
		i++
	}

	return newWorld
}

//CaluclateAliveCells return a slice of all the cells that are alive.
func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	aliveCells := []util.Cell{}

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == alive {
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}

	return aliveCells
}

//NewSlice creates a new slice.
func NewSlice(p Params) [][]byte {
	NS := make([][]byte, p.ImageHeight)
	for i := range NS {
		NS[i] = make([]byte, p.ImageWidth)
	}
	return NS
}

func countCells(p Params, world [][]byte) int {
	count := 0
	for Y := 0; Y < p.ImageHeight; Y++ {
		for X := 0; X < p.ImageWidth; X++ {
			if world[Y][X] != 0 {
				count++
			}
		}
	}
	return count
}

func worker(StartY, EndY int, p Params, world [][]byte, turn int, c distributorChannels, out chan<- [][]byte) {

	worldPart := calculateNextState(p, StartY, EndY, world, c, turn)
	out <- worldPart
}

func keyPress(p Params, c distributorChannels, turns int, world [][]byte, s chan State) {
	paused := false
	for {
		select {
		case keypressed := <-c.keyPresses:
			switch keypressed {
			case 's':
				generate(p, c, world)
			case 'p':
				if paused == true {
					s <- Executing
					paused = false
					fmt.Println("Continuing")
				} else {
					s <- Paused
					paused = true
				}
			case 'q':
				s <- Quitting
			}
		}

	}

}

func generate(p Params, c distributorChannels, world [][]byte) {

	c.ioCommand <- ioOutput
	filename1 := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)
	c.ioFilename <- filename1
	for Y := 0; Y < p.ImageHeight; Y++ {
		for X := 0; X < p.ImageWidth; X++ {
			c.ioImageOutput <- world[Y][X]
		}
	}

}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Create a 2D slice to store the world.
	World := NewSlice(p)

	//requests io goroutine to read the pgm given and start sending the bytes down the ioImageInput channel.
	c.ioCommand <- ioInput

	//Read in the parameters in order to create a filename and then send the filename down the filename channel.
	filename := fmt.Sprintf("%vx%v", p.ImageWidth, p.ImageHeight)
	c.ioFilename <- filename

	//Read the image byte by byte
	for Y := 0; Y < p.ImageHeight; Y++ {
		for X := 0; X < p.ImageWidth; X++ {
			i := <-c.ioImageInput
			if i != 0 {
				World[Y][X] = i
			}
		}
	}

	// TODO: For all initially alive cells send a CellFlipped Event.
	initialAliveCells := calculateAliveCells(p, World)
	for _, cell := range initialAliveCells {
		flipped := CellFlipped{
			CompletedTurns: 0,
			Cell:           cell,
		}
		c.events <- flipped
	}
	//initialise required variables and channels
	turn := 0
	aliveCells := countCells(p, World)
	ticker := time.NewTicker(2 * time.Second)
	s := make(chan State)
	done := make(chan bool)

	//start keypress goroutine which listens for keypresses sent down the channel and then sends the required state response down the s channel.
	go keyPress(p, c, turn, World, s)

	go func() {
		for {
			select {
			case <-ticker.C:
				cellCount := AliveCellsCount{
					CompletedTurns: turn,
					CellsCount:     aliveCells,
				}
				c.events <- cellCount
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	workerRows := p.ImageHeight / p.Threads
	remaining := p.ImageHeight % p.Threads
	out := make([]chan [][]byte, p.Threads)
	for i := range out {
		out[i] = make(chan [][]byte)
	}

	state := Executing
	{
	L:
		for j := 0; j < p.Turns; j++ {
		L1:
			for {
				select {
				case state = <-s:
					if state == Quitting {
						break L
					}
				default:
					if state == Paused {
						break
					}
					if p.Threads > 1 {
						t := turn + 1
						for i := 0; i < p.Threads; i++ {
							if (remaining > 0) && ((i + 1) == p.Threads) {
								go worker(i*workerRows, (((i + 1) * workerRows) + remaining), p, World, t, c, out[i])
							} else {
								go worker(i*workerRows, (i+1)*workerRows, p, World, t, c, out[i])
							}
						}

						newWorld := make([][]byte, 0)
						for i := 0; i < p.Threads; i++ {
							part := <-out[i]
							newWorld = append(newWorld, part...)
						}
						for Y := 0; Y < p.ImageHeight; Y++ {
							for X := 0; X < p.ImageWidth; X++ {
								World[Y][X] = newWorld[Y][X]
							}
						}

						turn++
						aliveCells = countCells(p, World)

						turncompleted := TurnComplete{
							CompletedTurns: turn,
						}
						c.events <- turncompleted
						break L1
					} else {
						t := turn + 1
						start := 0
						end := p.ImageHeight

						World = calculateNextState(p, start, end, World, c, t)

						turn++
						aliveCells = countCells(p, World)

						turncompleted := TurnComplete{
							CompletedTurns: turn,
						}
						c.events <- turncompleted
						break L1
					}
				}
			}

		}
	}

	// TODO: Execute all turns of the Game of Life.

	// TODO: Send correct Events when required, e.g. CellFlipped, TurnComplete and FinalTurnComplete.
	//		 See event.go for a list of all events.

	c.ioCommand <- ioOutput
	filename1 := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)
	c.ioFilename <- filename1
	for Y := 0; Y < p.ImageHeight; Y++ {
		for X := 0; X < p.ImageWidth; X++ {
			c.ioImageOutput <- World[Y][X]
		}
	}

	//send an event that lets the gui know the output has been sent.
	outputcompleted := ImageOutputComplete{
		CompletedTurns: turn,
		Filename:       filename,
	}
	c.events <- outputcompleted

	//send an event alerting that the final turn has been completed.
	finalCells := calculateAliveCells(p, World)
	final := FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          finalCells,
	}
	c.events <- final

	done <- true
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	//Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)

}
