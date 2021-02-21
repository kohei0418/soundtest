package main

import (
	"flag"
	"io"
	"math"
	"sync"
	"time"

	"github.com/hajimehoshi/oto"
)

var (
	sampleRate      = flag.Int("samplerate", 44100, "sample rate")
	channelNum      = flag.Int("channelnum", 2, "number of channel")
	bitDepthInBytes = flag.Int("bitdepthinbytes", 2, "bit depth in bytes")
)

const (
	SINE = iota
	TRIANGLE
	PULSE
)

type Wave struct {
	freq     float64
	length   int64
	pos      int64
	waveType int

	remaining []byte
}

func NewWave(freq float64, duration time.Duration, waveType int) *Wave {
	l := int64(*channelNum) * int64(*bitDepthInBytes) * int64(*sampleRate) * int64(duration) / int64(time.Second)
	l = l / 4 * 4
	return &Wave{
		freq:     freq,
		length:   l,
		waveType: waveType,
	}
}

func (s *Wave) Read(buf []byte) (int, error) {
	if len(s.remaining) > 0 {
		n := copy(buf, s.remaining)
		s.remaining = s.remaining[n:]
		return n, nil
	}

	if s.pos == s.length {
		return 0, io.EOF
	}

	eof := false
	if s.pos+int64(len(buf)) > s.length {
		buf = buf[:s.length-s.pos]
		eof = true
	}

	var origBuf []byte
	if len(buf)%4 > 0 {
		origBuf = buf
		buf = make([]byte, len(origBuf)+4-len(origBuf)%4)
	}

	length := float64(*sampleRate) / float64(s.freq)

	num := (*bitDepthInBytes) * (*channelNum)
	p := s.pos / int64(num)
	//intensity := float64(s.length - s.pos) / float64(s.length)

	const maxVol = 0.1

	switch *bitDepthInBytes {
	case 1:
		for i := 0; i < len(buf)/num; i++ {
			const max = 127

			var b int
			switch s.waveType {
			case PULSE:
				phase := p % int64(length)
				if phase < int64(length)/2 {
					b = max / 10 * 3
				} else {
					b = 0
				}
			case TRIANGLE:
				phase := float64(p % int64(length/2))
				if (p/(int64(length)/2))%2 == 0 {
					// going down
					b = int((1 - phase/(length/4)) * max * maxVol)
				} else {
					// going up
					b = int((phase/(length/4) - 1) * max * maxVol)
				}
			case SINE:
				b = int(math.Sin(2*math.Pi*float64(p)/length) * maxVol * max)
			}

			for ch := 0; ch < *channelNum; ch++ {
				buf[num*i+ch] = byte(b + 128)
			}
			p++
		}
	case 2:
		for i := 0; i < len(buf)/num; i++ {
			const max = 32767

			var b int
			switch s.waveType {
			case PULSE:
				phase := p % int64(length)
				if phase < int64(length)/2 {
					b = max / 10 * 3
				} else {
					b = 0
				}
			case TRIANGLE:
				phase := float64(p % int64(length/2))
				if (p/(int64(length)/2))%2 == 0 {
					// going down
					b = int((1 - phase/(length/4)) * max * maxVol * math.Sqrt(3))
				} else {
					// going up
					b = int((phase/(length/4) - 1) * max * maxVol * math.Sqrt(3))
				}
			case SINE:
				b = int(math.Sin(2*math.Pi*float64(p)/length) * maxVol * max * math.Sqrt2)
			}

			for ch := 0; ch < *channelNum; ch++ {
				buf[num*i+2*ch] = byte(b)
				buf[num*i+1+2*ch] = byte(b >> 8)
			}
			p++
		}
	}

	s.pos += int64(len(buf))

	n := len(buf)
	if origBuf != nil {
		n = copy(origBuf, buf)
		s.remaining = buf[n:]
	}

	if eof {
		return n, io.EOF
	}
	return n, nil
}

type Note struct {
	mute     bool
	freq     float64
	duration time.Duration
}

func play(context *oto.Context, freq float64, duration time.Duration, waveType int) error {
	p := context.NewPlayer()
	s := NewWave(freq, duration, waveType)
	if _, err := io.Copy(p, s); err != nil {
		return err
	}
	if err := p.Close(); err != nil {
		return err
	}
	return nil
}

func playPart(context *oto.Context, part *sync.WaitGroup, freq float64, duration time.Duration, waveType int) {
	part.Add(1)
	go func() {
		defer part.Done()
		play(context, freq, duration, waveType)
	}()
	part.Wait()
}

func mutePart(part *sync.WaitGroup, duration time.Duration) {
	part.Add(1)
	go func() {
		defer part.Done()
		time.Sleep(duration)
	}()
	part.Wait()
}

func playNotes(context *oto.Context, part *sync.WaitGroup, waveType int, notes []Note) {
	for _, note := range notes {
		if note.mute {
			mutePart(part, note.duration)
		} else {
			playPart(context, part, note.freq, note.duration, waveType)
		}
	}
}

func run() error {
	const (
		C = 523.3
		D = 587.3
		E = 659.3
		F = 698.5
		G = 784.0
		A = 880.1
	)
	const oneSecond = 1 * time.Second
	const halfSecond = oneSecond / 2

	c, err := oto.NewContext(*sampleRate, *channelNum, *bitDepthInBytes, 4096)
	if err != nil {
		return err
	}

	var band sync.WaitGroup

	var part1 sync.WaitGroup
	var part2 sync.WaitGroup
	var part3 sync.WaitGroup

	notes := []Note{
		{freq: C, duration: oneSecond},
		{freq: D, duration: oneSecond},
		{freq: E, duration: oneSecond},
		{freq: F, duration: oneSecond},

		{freq: E, duration: oneSecond},
		{freq: D, duration: oneSecond},
		{freq: C, duration: oneSecond},
		{mute: true, duration: oneSecond},

		{freq: E, duration: oneSecond},
		{freq: F, duration: oneSecond},
		{freq: G, duration: oneSecond},
		{freq: A, duration: oneSecond},

		{freq: G, duration: oneSecond},
		{freq: F, duration: oneSecond},
		{freq: E, duration: oneSecond},
		{mute: true, duration: oneSecond},

		{freq: C, duration: oneSecond},
		{mute: true, duration: oneSecond},
		{freq: C, duration: oneSecond},
		{mute: true, duration: oneSecond},
		{freq: C, duration: oneSecond},
		{mute: true, duration: oneSecond},
		{freq: C, duration: oneSecond},
		{mute: true, duration: oneSecond},

		{freq: C, duration: halfSecond},
		{freq: C, duration: halfSecond},
		{freq: D, duration: halfSecond},
		{freq: D, duration: halfSecond},
		{freq: E, duration: halfSecond},
		{freq: E, duration: halfSecond},
		{freq: F, duration: halfSecond},
		{freq: F, duration: halfSecond},

		{freq: E, duration: halfSecond},
		{mute: true, duration: halfSecond},
		{freq: D, duration: halfSecond},
		{mute: true, duration: halfSecond},
		{freq: C, duration: halfSecond},
		{mute: true, duration: halfSecond + oneSecond},
	}

	band.Add(1)
	go func() {
		defer band.Done()
		playNotes(c, &part1, SINE, notes)
	}()

	band.Add(1)
	go func() {
		defer band.Done()
		time.Sleep(oneSecond * 4)
		playNotes(c, &part2, TRIANGLE, notes)
	}()

	band.Add(1)
	go func() {
		defer band.Done()
		time.Sleep(oneSecond * 8)
		playNotes(c, &part3, PULSE, notes)
	}()

	band.Wait()

	c.Close()
	return nil
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		panic(err)
	}
}
