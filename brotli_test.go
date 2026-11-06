// Copyright 2016 Google Inc. All Rights Reserved.
//
// Distributed under MIT license.
// See file LICENSE for detail or copy at https://opensource.org/licenses/MIT

package brotli

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/andybalholm/brotli/matchfinder"
	"github.com/xyproto/randomstring"
)

func checkCompressedData(compressedData, wantOriginalData []byte) error {
	uncompressed, err := Decode(compressedData)
	if err != nil {
		return fmt.Errorf("brotli decompress failed: %v", err)
	}
	if !bytes.Equal(uncompressed, wantOriginalData) {
		if len(wantOriginalData) != len(uncompressed) {
			return fmt.Errorf(""+
				"Data doesn't uncompress to the original value.\n"+
				"Length of original: %v\n"+
				"Length of uncompressed: %v",
				len(wantOriginalData), len(uncompressed))
		}
		for i := range wantOriginalData {
			if wantOriginalData[i] != uncompressed[i] {
				return fmt.Errorf(""+
					"Data doesn't uncompress to the original value.\n"+
					"Original at %v is %v\n"+
					"Uncompressed at %v is %v",
					i, wantOriginalData[i], i, uncompressed[i])
			}
		}
	}
	return nil
}

func TestEncoderNoWrite(t *testing.T) {
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	if err := e.Close(); err != nil {
		t.Errorf("Close()=%v, want nil", err)
	}
	// Check Write after close.
	if _, err := e.Write([]byte("hi")); err == nil {
		t.Errorf("No error after Close() + Write()")
	}
}

func TestEncoderEmptyWrite(t *testing.T) {
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	n, err := e.Write([]byte(""))
	if n != 0 || err != nil {
		t.Errorf("Write()=%v,%v, want 0, nil", n, err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close()=%v, want nil", err)
	}
}
func TestIssue22(t *testing.T) {
	f, err := os.Open("testdata/issue22.gz")
	if err != nil {
		t.Fatalf("Error opening test data file: %v", err)
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("Error creating gzip reader: %v", err)
	}

	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("Error reading test data: %v", err)
	}

	if len(data) != 2851073 {
		t.Fatalf("Wrong length for test data: got %d, want 2851073", len(data))
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		out := bytes.Buffer{}
		e := NewWriterOptions(&out, WriterOptions{Quality: level})
		n, err := e.Write(data)
		if err != nil {
			t.Errorf("Error compressing data: %v", err)
		}
		if int(n) != len(data) {
			t.Errorf("Write() n=%v, want %v", n, len(data))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close Error after writing %d bytes: %v", n, err)
		}
		if err := checkCompressedData(out.Bytes(), data); err != nil {
			t.Errorf("Error decompressing data at level %d: %v", level, err)
		}
	}
}

func TestWriterV2(t *testing.T) {
	for level := BestSpeed; level <= BestCompression; level++ {
		// Test basic encoder usage.
		input := []byte("<html><body><H1>Hello world</H1></body></html>")
		out := bytes.Buffer{}
		e := NewWriterV2(&out, level)
		in := bytes.NewReader([]byte(input))
		n, err := io.Copy(e, in)
		if err != nil {
			t.Errorf("Copy Error: %v", err)
		}
		if int(n) != len(input) {
			t.Errorf("Copy() n=%v, want %v", n, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close Error after copied %d bytes: %v", n, err)
		}
		if err := checkCompressedData(out.Bytes(), input); err != nil {
			t.Error(err)
		}

		out2 := bytes.Buffer{}
		e.Reset(&out2)
		n2, err := e.Write(input)
		if err != nil {
			t.Errorf("Write error after Reset: %v", err)
		}
		if n2 != len(input) {
			t.Errorf("Write() after Reset n=%d, want %d", n2, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close error after Reset (copied %d) bytes: %v", n2, err)
		}
		if !bytes.Equal(out.Bytes(), out2.Bytes()) {
			t.Error("Compressed data after Reset doesn't equal first time")
		}
	}
}

func TestEncoderStreams(t *testing.T) {
	// Test that output is streamed.
	// Adjust window size to ensure the encoder outputs at least enough bytes
	// to fill the window.
	const lgWin = 16
	windowSize := int(math.Pow(2, lgWin))
	input := make([]byte, 8*windowSize)
	rand.Read(input)
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 11, LGWin: lgWin})
	halfInput := input[:len(input)/2]
	in := bytes.NewReader(halfInput)

	n, err := io.Copy(e, in)
	if err != nil {
		t.Errorf("Copy Error: %v", err)
	}

	// We've fed more data than the sliding window size. Check that some
	// compressed data has been output.
	if out.Len() == 0 {
		t.Errorf("Output length is 0 after %d bytes written", n)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close Error after copied %d bytes: %v", n, err)
	}
	if err := checkCompressedData(out.Bytes(), halfInput); err != nil {
		t.Error(err)
	}
}

func TestEncoderLargeInput(t *testing.T) {
	for level := BestSpeed; level <= BestCompression; level++ {
		input := make([]byte, 1000000)
		rand.Read(input)
		out := bytes.Buffer{}
		e := NewWriterOptions(&out, WriterOptions{Quality: level})
		in := bytes.NewReader(input)

		n, err := io.Copy(e, in)
		if err != nil {
			t.Errorf("Copy Error: %v", err)
		}
		if int(n) != len(input) {
			t.Errorf("Copy() n=%v, want %v", n, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close Error after copied %d bytes: %v", n, err)
		}
		if err := checkCompressedData(out.Bytes(), input); err != nil {
			t.Error(err)
		}

		out2 := bytes.Buffer{}
		e.Reset(&out2)
		n2, err := e.Write(input)
		if err != nil {
			t.Errorf("Write error after Reset: %v", err)
		}
		if n2 != len(input) {
			t.Errorf("Write() after Reset n=%d, want %d", n2, len(input))
		}
		if err := e.Close(); err != nil {
			t.Errorf("Close error after Reset (copied %d) bytes: %v", n2, err)
		}
		if !bytes.Equal(out.Bytes(), out2.Bytes()) {
			t.Error("Compressed data after Reset doesn't equal first time")
		}
	}
}

func TestEncoderFlush(t *testing.T) {
	input := make([]byte, 1000)
	rand.Read(input)
	out := bytes.Buffer{}
	e := NewWriterOptions(&out, WriterOptions{Quality: 5})
	in := bytes.NewReader(input)
	_, err := io.Copy(e, in)
	if err != nil {
		t.Fatalf("Copy Error: %v", err)
	}
	if err := e.Flush(); err != nil {
		t.Fatalf("Flush(): %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("0 bytes written after Flush()")
	}
	decompressed := make([]byte, 1000)
	reader := NewReader(bytes.NewReader(out.Bytes()))
	n, err := reader.Read(decompressed)
	if n != len(decompressed) || err != nil {
		t.Errorf("Expected <%v, nil>, but <%v, %v>", len(decompressed), n, err)
	}
	if !bytes.Equal(decompressed, input) {
		t.Errorf(""+
			"Decompress after flush: %v\n"+
			"%q\n"+
			"want:\n%q",
			err, decompressed, input)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close(): %v", err)
	}
}

type readerWithTimeout struct {
	io.Reader
}

func (r readerWithTimeout) Read(p []byte) (int, error) {
	type result struct {
		n   int
		err error
	}
	ch := make(chan result)
	go func() {
		n, err := r.Reader.Read(p)
		ch <- result{n, err}
	}()
	select {
	case result := <-ch:
		return result.n, result.err
	case <-time.After(5 * time.Second):
		return 0, fmt.Errorf("read timed out")
	}
}

func TestDecoderStreaming(t *testing.T) {
	pr, pw := io.Pipe()
	writer := NewWriterOptions(pw, WriterOptions{Quality: 5, LGWin: 20})
	reader := readerWithTimeout{NewReader(pr)}
	defer func() {
		go ioutil.ReadAll(pr) // swallow the "EOF" token from writer.Close
		if err := writer.Close(); err != nil {
			t.Errorf("writer.Close: %v", err)
		}
	}()

	ch := make(chan []byte)
	errch := make(chan error)
	go func() {
		for {
			segment, ok := <-ch
			if !ok {
				return
			}
			if n, err := writer.Write(segment); err != nil || n != len(segment) {
				errch <- fmt.Errorf("write=%v,%v, want %v,%v", n, err, len(segment), nil)
				return
			}
			if err := writer.Flush(); err != nil {
				errch <- fmt.Errorf("flush: %v", err)
				return
			}
		}
	}()
	defer close(ch)

	segments := [...][]byte{
		[]byte("first"),
		[]byte("second"),
		[]byte("third"),
	}
	for k, segment := range segments {
		t.Run(fmt.Sprintf("Segment%d", k), func(t *testing.T) {
			select {
			case ch <- segment:
			case err := <-errch:
				t.Fatalf("write: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out")
			}
			wantLen := len(segment)
			got := make([]byte, wantLen)
			if n, err := reader.Read(got); err != nil || n != wantLen || !bytes.Equal(got, segment) {
				t.Fatalf("read[%d]=%q,%v,%v, want %q,%v,%v", k, got, n, err, segment, wantLen, nil)
			}
		})
	}
}

func TestReader(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	r := NewReader(bytes.NewReader(encoded))
	var decodedOutput bytes.Buffer
	n, err := io.Copy(&decodedOutput, r)
	if err != nil {
		t.Fatalf("Copy(): n=%v, err=%v", n, err)
	}
	if got := decodedOutput.Bytes(); !bytes.Equal(got, content) {
		t.Errorf(""+
			"Reader output:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			got, len(content))
	}

	r.Reset(bytes.NewReader(encoded))
	decodedOutput.Reset()
	n, err = io.Copy(&decodedOutput, r)
	if err != nil {
		t.Fatalf("After Reset: Copy(): n=%v, err=%v", n, err)
	}
	if got := decodedOutput.Bytes(); !bytes.Equal(got, content) {
		t.Errorf("After Reset: "+
			"Reader output:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			got, len(content))
	}

}

func TestDecode(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	decoded, err := Decode(encoded)
	if err != nil {
		t.Errorf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, content) {
		t.Errorf(""+
			"Decode content:\n"+
			"%q\n"+
			"want:\n"+
			"<%d bytes>",
			decoded, len(content))
	}
}

func TestQuality(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 10000)
	for q := 0; q < 12; q++ {
		encoded, _ := Encode(content, WriterOptions{Quality: q})
		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode: %v", err)
		}
		if !bytes.Equal(decoded, content) {
			t.Errorf(""+
				"Decode content:\n"+
				"%q\n"+
				"want:\n"+
				"<%d bytes>",
				decoded, len(content))
		}
	}
}

func TestDecodeFuzz(t *testing.T) {
	// Test that the decoder terminates with corrupted input.
	content := bytes.Repeat([]byte("hello world!"), 100)
	rnd := rand.New(rand.NewSource(0))
	encoded, err := Encode(content, WriterOptions{Quality: 5})
	if err != nil {
		t.Fatalf("Encode(<%d bytes>, _) = _, %s", len(content), err)
	}
	if len(encoded) == 0 {
		t.Fatalf("Encode(<%d bytes>, _) produced empty output", len(content))
	}
	for i := 0; i < 100; i++ {
		enc := append([]byte{}, encoded...)
		for j := 0; j < 5; j++ {
			enc[rnd.Intn(len(enc))] = byte(rnd.Intn(256))
		}
		Decode(enc)
	}
}

func TestDecodeTrailingData(t *testing.T) {
	content := bytes.Repeat([]byte("hello world!"), 100)
	encoded, _ := Encode(content, WriterOptions{Quality: 5})
	_, err := Decode(append(encoded, 0))
	if err == nil {
		t.Errorf("Expected 'excessive input' error")
	}
}

func TestEncodeDecode(t *testing.T) {
	for _, test := range []struct {
		data    []byte
		repeats int
	}{
		{nil, 0},
		{[]byte("A"), 1},
		{[]byte("<html><body><H1>Hello world</H1></body></html>"), 10},
		{[]byte("<html><body><H1>Hello world</H1></body></html>"), 1000},
	} {
		t.Logf("case %q x %d", test.data, test.repeats)
		input := bytes.Repeat(test.data, test.repeats)
		encoded, err := Encode(input, WriterOptions{Quality: 5})
		if err != nil {
			t.Errorf("Encode: %v", err)
		}
		// Inputs are compressible, but may be too small to compress.
		if maxSize := len(input)/2 + 20; len(encoded) >= maxSize {
			t.Errorf(""+
				"Encode returned %d bytes, want <%d\n"+
				"Encoded=%q",
				len(encoded), maxSize, encoded)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Decode: %v", err)
		}
		if !bytes.Equal(decoded, input) {
			var want string
			if len(input) > 320 {
				want = fmt.Sprintf("<%d bytes>", len(input))
			} else {
				want = fmt.Sprintf("%q", input)
			}
			t.Errorf(""+
				"Decode content:\n"+
				"%q\n"+
				"want:\n"+
				"%s",
				decoded, want)
		}
	}
}

func TestErrorReset(t *testing.T) {
	compress := func(input []byte) []byte {
		var buf bytes.Buffer
		writer := new(Writer)
		writer.Reset(&buf)
		writer.Write(input)
		writer.Close()

		return buf.Bytes()
	}

	corruptReader := func(reader *Reader) {
		buf := bytes.NewBuffer([]byte("trash"))
		reader.Reset(buf)
		_, err := io.ReadAll(reader)
		if err == nil {
			t.Fatalf("successively decompressed invalid input")
		}
	}

	decompress := func(input []byte, reader *Reader) []byte {
		buf := bytes.NewBuffer(input)
		reader.Reset(buf)
		output, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("failed to decompress data %s", err.Error())
		}

		return output
	}

	source := []byte("text")

	compressed := compress(source)
	reader := new(Reader)
	corruptReader(reader)
	decompressed := decompress(compressed, reader)
	if string(source) != string(decompressed) {
		t.Fatalf("decompressed data does not match original state")
	}
}

// Encode returns content encoded with Brotli.
func Encode(content []byte, options WriterOptions) ([]byte, error) {
	var buf bytes.Buffer
	writer := NewWriterOptions(&buf, options)
	_, err := writer.Write(content)
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	return buf.Bytes(), err
}

// Decode decodes Brotli encoded data.
func Decode(encodedData []byte) ([]byte, error) {
	r := NewReader(bytes.NewReader(encodedData))
	return ioutil.ReadAll(r)
}

func BenchmarkEncodeLevels(b *testing.B) {
	opticks, err := ioutil.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(opticks)))
			for i := 0; i < b.N; i++ {
				w := NewWriterLevel(ioutil.Discard, level)
				w.Write(opticks)
				w.Close()
			}
		})
	}
}

func BenchmarkEncodeLevelsReset(b *testing.B) {
	opticks, err := ioutil.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		buf := new(bytes.Buffer)
		w := NewWriterLevel(buf, level)
		w.Write(opticks)
		w.Close()
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(len(opticks))/float64(buf.Len()), "ratio")
			b.SetBytes(int64(len(opticks)))
			for i := 0; i < b.N; i++ {
				w.Reset(ioutil.Discard)
				w.Write(opticks)
				w.Close()
			}
		})
	}
}

func BenchmarkEncodeLevelsResetV2(b *testing.B) {
	opticks, err := ioutil.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= 9; level++ {
		buf := new(bytes.Buffer)
		w := NewWriterV2(buf, level)
		w.Write(opticks)
		w.Close()
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(len(opticks))/float64(buf.Len()), "ratio")
			b.SetBytes(int64(len(opticks)))
			for i := 0; i < b.N; i++ {
				w.Reset(ioutil.Discard)
				w.Write(opticks)
				w.Close()
			}
		})
	}
}

func BenchmarkDecodeLevels(b *testing.B) {
	opticks, err := ioutil.ReadFile("testdata/Isaac.Newton-Opticks.txt")
	if err != nil {
		b.Fatal(err)
	}

	for level := BestSpeed; level <= BestCompression; level++ {
		buf := new(bytes.Buffer)
		w := NewWriterLevel(buf, level)
		w.Write(opticks)
		w.Close()
		compressed := buf.Bytes()
		b.Run(fmt.Sprintf("%d", level), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(opticks)))
			for i := 0; i < b.N; i++ {
				io.Copy(ioutil.Discard, NewReader(bytes.NewReader(compressed)))
			}
		})
	}
}

func test(t *testing.T, filename string, m matchfinder.MatchFinder, blockSize int) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	b := new(bytes.Buffer)
	w := &matchfinder.Writer{
		Dest:        b,
		MatchFinder: m,
		Encoder:     &Encoder{},
		BlockSize:   blockSize,
	}
	w.Write(data)
	w.Close()
	compressed := b.Bytes()
	sr := NewReader(bytes.NewReader(compressed))
	decompressed, err := ioutil.ReadAll(sr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decompressed, data) {
		t.Fatal("decompressed output doesn't match")
	}
}

func benchmark(b *testing.B, filename string, m matchfinder.MatchFinder, blockSize int) {
	b.StopTimer()
	b.ReportAllocs()
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(data)))
	buf := new(bytes.Buffer)
	w := &matchfinder.Writer{
		Dest:        buf,
		MatchFinder: m,
		Encoder:     &Encoder{},
		BlockSize:   blockSize,
	}
	w.Write(data)
	w.Close()
	b.ReportMetric(float64(len(data))/float64(buf.Len()), "ratio")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		w.Reset(ioutil.Discard)
		w.Write(data)
		w.Close()
	}
}

func TestEncodeM4(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 18, DistanceBitCost: 66}, 1<<16)
}

func TestEncodeM4Chain256(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 18, DistanceBitCost: 66, ChainLength: 256}, 1<<16)
}

func BenchmarkEncodeM4(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, DistanceBitCost: 66}, 1<<16)
}

func TestEncodeM4Chain1(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 18, ChainLength: 1, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain1(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 1, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain2(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 2, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain4(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 4, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain8(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 8, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain16(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 16, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain32(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 32, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain64(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 64, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain128(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 128, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func BenchmarkEncodeM4Chain256(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.M4{MaxDistance: 1 << 20, ChainLength: 256, HashLen: 5, DistanceBitCost: 66}, 1<<16)
}

func TestEncodePathfinder(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 18}, 1<<16)
}

func TestEncodePathfinderChain256(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 18, ChainLength: 256}, 1<<16)
}

func BenchmarkEncodePathfinder(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20}, 1<<16)
}

func TestEncodePathfinderChain1(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 18, ChainLength: 1}, 1<<16)
}

func BenchmarkEncodePathfinderChain1(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 1}, 1<<16)
}

func BenchmarkEncodePathfinderChain2(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 2}, 1<<16)
}

func BenchmarkEncodePathfinderChain4(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 4}, 1<<16)
}

func BenchmarkEncodePathfinderChain8(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 8, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain16(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 16, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain32(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 32, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain64(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 64, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain128(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 128, HashLen: 5}, 1<<16)
}

func BenchmarkEncodePathfinderChain256(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", &matchfinder.Pathfinder{MaxDistance: 1 << 20, ChainLength: 256, HashLen: 4, MinLength: 3}, 1<<16)
}

func TestEncodeM0(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", matchfinder.M0{}, 1<<16)
}

func BenchmarkEncodeM0(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", matchfinder.M0{}, 1<<16)
}

func TestEncodeM0Lazy(t *testing.T) {
	test(t, "testdata/Isaac.Newton-Opticks.txt", matchfinder.M0{Lazy: true}, 1<<16)
}

func BenchmarkEncodeM0Lazy(b *testing.B) {
	benchmark(b, "testdata/Isaac.Newton-Opticks.txt", matchfinder.M0{Lazy: true}, 1<<16)
}

func TestIssue51(t *testing.T) {
	for i := 65536; i <= 65536*4; i += 65536 {
		t.Run("compress data length: "+strconv.Itoa(i)+"bytes", func(t *testing.T) {
			dataStr := randomstring.HumanFriendlyString(i)
			dataBytes := []byte(dataStr)
			buf := bytes.Buffer{}
			w := NewWriterV2(&buf, 4)

			n, err := w.Write(dataBytes)
			if err != nil {
				t.Fatalf("Error while compressing data: %v", err)
			}
			if n != len(dataBytes) {
				t.Fatalf("Bytes written (%d) != len(databytes) (%d)", n, len(dataBytes))
			}
			err = w.Close()
			if err != nil {
				t.Fatalf("Error closing writer: %v", err)
			}

			r := NewReader(&buf)
			dst := make([]byte, len(dataBytes)+100)
			p := dst
			total := 0
			for {
				n1, err1 := r.Read(p)
				if err1 != nil {
					if err1 != io.EOF {
						t.Fatal(err1)
					}
					break
				}
				total += n1
				p = p[n1:]
			}
			if !bytes.Equal(dst[:total], dataBytes) {
				t.Fatal("Decompressed bytes don't match")
			}
		})
	}
}

func TestIssue58(t *testing.T) {
	content := []byte("---\nthis-is-not-brotli: \"it is actually yaml\"")
	input := bytes.NewBuffer(content)

	r := NewReader(input)

	buf, err := io.ReadAll(r)
	if err == nil {
		t.Fatalf("expected error, got none and read:\n%x\n%s\n%v", buf, buf, buf)
	}
}

// testBitWriter packs bits LSB-first, the way brotli streams are laid out.
type testBitWriter struct {
	buf  []byte
	nbit uint
}

func (w *testBitWriter) writeBits(n uint, v uint32) {
	for i := uint(0); i < n; i++ {
		for uint(len(w.buf)) <= w.nbit>>3 {
			w.buf = append(w.buf, 0)
		}
		if v>>i&1 != 0 {
			w.buf[w.nbit>>3] |= 1 << (w.nbit & 7)
		}
		w.nbit++
	}
}

// writeZeroBits skips n bits, leaving them zero. It must only be followed by
// more zero bits.
func (w *testBitWriter) writeZeroBits(n uint) {
	w.nbit += n
}

// writeSingleSymbolCode writes a simple prefix code with a single symbol;
// decoding that symbol consumes no bits at all.
func (w *testBitWriter) writeSingleSymbolCode(alphabetBits uint, symbol uint32) {
	w.writeBits(2, 1) // HSKIP = 1: simple prefix code
	w.writeBits(2, 0) // NSYM - 1 = 0
	w.writeBits(alphabetBits, symbol)
}

// bytes returns the stream padded to a byte boundary with zero bits, followed
// by trailing zero bytes.
func (w *testBitWriter) bytes(trailing int) []byte {
	out := make([]byte, int((w.nbit+7)/8)+trailing)
	copy(out, w.buf)
	return out
}

const (
	// A category that declares a single block type keeps the initial block
	// length of a meta-block, 1<<24, see decoderStateMetablockBegin.
	invalidSwitchBlockLength = 1 << 24

	// Insert-and-copy symbol 130: no literals, copy length 4, explicit
	// distance.
	invalidSwitchEmptyCommand = 130

	// With NPOSTFIX = 3 and NDIRECT = 120, distance symbol 304 is followed by
	// 11 (zero) extra bits and decodes to distance 49241. At output position 0
	// that is a static dictionary reference to a 4-byte word with transform 48
	// (OmitLast7), which produces no output. Such commands never complete the
	// meta-block, so a stream can hold as many of them as it likes.
	invalidSwitchEmptyDistance  = 304
	invalidSwitchDistanceExtras = 11
)

// writeInvalidSwitchMetablockStart writes the stream header and the start of
// a last meta-block with MLEN = 1.
func writeInvalidSwitchMetablockStart(w *testBitWriter) {
	w.writeBits(1, 0)  // WBITS = 16
	w.writeBits(1, 1)  // ISLAST
	w.writeBits(1, 0)  // ISLASTEMPTY = 0
	w.writeBits(2, 0)  // MNIBBLES = 4
	w.writeBits(16, 0) // MLEN - 1 = 0
}

// writeInvalidSwitchPrefixCodes writes the rest of the meta-block header after
// the block type declarations: distance parameters, a single literal and
// distance tree, and one single-symbol insert-and-copy code per command block
// type.
func writeInvalidSwitchPrefixCodes(w *testBitWriter, commandSymbols ...uint32) {
	w.writeBits(2, 3)  // NPOSTFIX = 3
	w.writeBits(4, 15) // NDIRECT = 15 << NPOSTFIX = 120
	w.writeBits(2, 0)  // context mode of the only literal block type
	w.writeBits(1, 0)  // NTREESL = 1
	w.writeBits(1, 0)  // NTREESD = 1
	w.writeSingleSymbolCode(8, 'A')
	for _, symbol := range commandSymbols {
		w.writeSingleSymbolCode(10, symbol)
	}
	w.writeSingleSymbolCode(10, invalidSwitchEmptyDistance)
}

// invalidCommandSwitchStream returns a stream whose only meta-block declares
// a single block type for every category, followed by exactly 1<<24 commands
// that produce no output, so decoding the next command needs a command block
// switch that the header never allowed. trailing zero bytes are appended.
// It also returns the offset of the byte holding the end of the last command.
func invalidCommandSwitchStream(trailing int) ([]byte, int) {
	w := &testBitWriter{}
	writeInvalidSwitchMetablockStart(w)
	w.writeBits(1, 0) // NBLTYPESL = 1
	w.writeBits(1, 0) // NBLTYPESI = 1
	w.writeBits(1, 0) // NBLTYPESD = 1
	writeInvalidSwitchPrefixCodes(w, invalidSwitchEmptyCommand)
	w.writeZeroBits(invalidSwitchBlockLength * invalidSwitchDistanceExtras)
	return w.bytes(trailing), int(w.nbit / 8)
}

// invalidDistanceSwitchStream returns a stream whose only meta-block declares
// two command block types but a single distance block type. The first command
// block is longer than 1<<24 commands, so the distance block length runs out
// while a command still needs a distance: an invalid distance block switch.
// Past that point the stream is well-formed: more commands that produce no
// output, a valid switch to the second command block type, and a command that
// inserts the literal 'A' and completes the meta-block. A decoder that ignores
// the invalid distance block switch accepts the stream and outputs "A".
// It also returns the offset of the byte where the invalid switch is reached.
func invalidDistanceSwitchStream() ([]byte, int) {
	const extraCommands = 256
	w := &testBitWriter{}
	writeInvalidSwitchMetablockStart(w)
	w.writeBits(1, 0) // NBLTYPESL = 1
	w.writeBits(1, 1) // NBLTYPESI = 2
	w.writeBits(3, 0)
	w.writeSingleSymbolCode(2, 1)  // block type code 1: previous type + 1
	w.writeSingleSymbolCode(5, 25) // block length code 25: 16625 + 24 bits
	w.writeBits(24, invalidSwitchBlockLength+extraCommands-16625)
	w.writeBits(1, 0) // NBLTYPESD = 1
	// Command block type 1 uses symbol 8: insert 1 literal, copy length 2,
	// implicit distance.
	writeInvalidSwitchPrefixCodes(w, invalidSwitchEmptyCommand, 8)
	w.writeZeroBits(invalidSwitchBlockLength * invalidSwitchDistanceExtras)
	critical := int(w.nbit / 8)
	w.writeZeroBits(extraCommands * invalidSwitchDistanceExtras)
	w.writeZeroBits(24) // command block switch: block length 16625
	// The last command and its literal consume no bits.
	return w.bytes(0), critical
}

// chunkedReader serves data in reads that end at offsets chosen by next. It
// lets a test decide whether the decoder reaches a point of the stream on its
// fast path (plenty of buffered input) or on its byte-wise safe path.
type chunkedReader struct {
	data []byte
	pos  int
	next func(pos int) int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	end := r.next(r.pos)
	if end > len(r.data) {
		end = len(r.data)
	}
	if end-r.pos > len(p) {
		end = r.pos + len(p)
	}
	n := copy(p, r.data[r.pos:end])
	r.pos += n
	return n, nil
}

// splitBefore returns a chunkedReader that ends one read at offset split and
// then serves the rest of data in reads of at most chunk bytes.
func splitBefore(data []byte, split, chunk int) *chunkedReader {
	return &chunkedReader{data: data, next: func(pos int) int {
		if pos < split {
			return split
		}
		return pos + chunk
	}}
}

// decodeWithDeadline decodes everything from src. A decoder that panics or
// never terminates fails the test instead of crashing or hanging it.
func decodeWithDeadline(t *testing.T, src io.Reader) ([]byte, error) {
	t.Helper()
	type result struct {
		out      []byte
		err      error
		panicked interface{}
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- result{panicked: p}
			}
		}()
		out, err := io.ReadAll(NewReader(src))
		done <- result{out: out, err: err}
	}()
	select {
	case r := <-done:
		if r.panicked != nil {
			t.Fatalf("decoder panicked: %v", r.panicked)
		}
		return r.out, r.err
	case <-time.After(2 * time.Minute):
		t.Fatalf("decoder did not terminate")
	}
	return nil, nil
}

// TestDecodeInvalidBlockSwitch checks that a block switch in a category that
// declares a single block type is reported as a format error. A meta-block
// reaches such a switch once it has used up the initial block length of
// 1<<24 (e.g. with commands that produce no output). The decoder used to
// ignore it, which made it spin forever on its fast path, overflow its
// internal input buffer on its safe path, and accept malformed streams.
func TestDecodeInvalidBlockSwitch(t *testing.T) {
	wantErr := decodeError(decoderErrorFormatBlockSwitch)
	if got := wantErr.Error(); got != "brotli: BLOCK_SWITCH" {
		t.Errorf("error message = %q, want %q", got, "brotli: BLOCK_SWITCH")
	}

	const trailing = 64
	stream, critical := invalidCommandSwitchStream(trailing)

	t.Run("CommandFastPath", func(t *testing.T) {
		// The invalid switch is reached with plenty of input buffered.
		src := splitBefore(stream, critical-4096, readBufSize)
		out, err := decodeWithDeadline(t, src)
		if err != wantErr || len(out) != 0 {
			t.Errorf("decode = %q, %v; want no output, %v", out, err, wantErr)
		}
	})

	t.Run("CommandSafePath", func(t *testing.T) {
		// The input arrives one byte at a time around the invalid switch.
		src := splitBefore(stream, critical-trailing, 1)
		out, err := decodeWithDeadline(t, src)
		if err != wantErr || len(out) != 0 {
			t.Errorf("decode = %q, %v; want no output, %v", out, err, wantErr)
		}
	})

	t.Run("CommandEndOfInput", func(t *testing.T) {
		// The stream ends right before the invalid switch: this is a
		// format error, not a truncated stream.
		src := bytes.NewReader(stream[:len(stream)-trailing])
		out, err := decodeWithDeadline(t, src)
		if err != wantErr || len(out) != 0 {
			t.Errorf("decode = %q, %v; want no output, %v", out, err, wantErr)
		}
	})

	t.Run("Distance", func(t *testing.T) {
		stream, critical := invalidDistanceSwitchStream()
		src := splitBefore(stream, critical-4096, readBufSize)
		out, err := decodeWithDeadline(t, src)
		if err != wantErr || len(out) != 0 {
			t.Errorf("decode = %q, %v; want no output, %v", out, err, wantErr)
		}
	})
}
