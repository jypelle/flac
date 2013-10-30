/*
Links:
	http://code.google.com/p/goflac-meta/source/browse/flacmeta_test.go
	http://flac.sourceforge.net/api/hierarchy.html
	http://flac.sourceforge.net/documentation_format_overview.html
	http://flac.sourceforge.net/format.html
	http://jflac.sourceforge.net/
	http://ffmpeg.org/doxygen/trunk/libavcodec_2flacdec_8c-source.html#l00485
	http://mi.eng.cam.ac.uk/reports/svr-ftp/auto-pdf/robinson_tr156.pdf
*/

// Package flac provides access to FLAC [1] (Free Lossless Audio Codec) files.
//
// The basic structure of a FLAC bitstream is:
//    - The four byte string "fLaC".
//    - The StreamInfo metadata block.
//    - Zero or more other metadata blocks.
//    - One or more audio frames.
//
// [1]: http://flac.sourceforge.net/format.html
package flac

import (
	"fmt"
	"io"
	"os"

	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

// A Stream is a FLAC bitstream.
type Stream struct {
	// The underlying reader of the stream.
	r io.ReadSeeker
	// Metadata blocks.
	MetaBlocks []*meta.Block
	// Audio frames.
	Frames []*frame.Frame
}

// Parse reads the provided file and returns a parsed FLAC bitstream. It parses
// all metadata blocks and all audio frames. Use Open instead for more
// granularity.
func Parse(filePath string) (s *Stream, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ParseStream(f)
}

// Open validates the FLAC signature of the provided file and returns a handle
// to the FLAC bitstream. Callers should close the stream when done reading from
// it. Call either Stream.Parse or Stream.ParseBlocks and Stream.ParseFrames to
// parse the metadata blocks and audio frames.
func Open(filePath string) (s *Stream, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	return NewStream(f)
}

// Close closes the underlying reader of the stream.
func (s *Stream) Close() error {
	r, ok := s.r.(io.Closer)
	if ok {
		return r.Close()
	}
	return nil
}

// ParseStream reads from the provided io.ReadSeeker and returns a parsed FLAC
// bitstream. It parses all metadata blocks and all audio frames.  Use NewStream
// instead for more granularity.
func ParseStream(r io.ReadSeeker) (s *Stream, err error) {
	s, err = NewStream(r)
	if err != nil {
		return nil, err
	}
	err = s.Parse()
	if err != nil {
		return nil, err
	}
	return s, nil
}

// FlacSignature is present at the beginning of each FLAC file.
const FlacSignature = "fLaC"

// NewStream validates the FLAC signature of the provided io.ReadSeeker and
// returns a handle to the FLAC bitstream. Call either Stream.Parse or
// Stream.ParseBlocks and Stream.ParseFrames to parse the metadata blocks and
// audio frames.
func NewStream(r io.ReadSeeker) (s *Stream, err error) {
	// Verify "fLaC" signature (size: 4 bytes).
	buf := make([]byte, 4)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	sig := string(buf)
	if sig != FlacSignature {
		return nil, fmt.Errorf("flac.NewStream: invalid signature; expected %q, got %q", FlacSignature, sig)
	}

	s = &Stream{r: r}
	return s, nil
}

// Parse reads and parses all metadata blocks and audio frames of the stream.
// Use Stream.ParseBlocks and Stream.ParseFrames instead for more granularity.
func (s *Stream) Parse() (err error) {
	err = s.ParseBlocks(meta.TypeAll)
	if err != nil {
		return err
	}
	err = s.ParseFrames()
	if err != nil {
		return err
	}
	return nil
}

// ParseBlocks reads and parses the specified metadata blocks of the stream,
// based on the provided types bitfield. The StreamInfo block type is always
// included.
func (s *Stream) ParseBlocks(types meta.BlockType) (err error) {
	// The StreamInfo block type is always included.
	types |= meta.TypeStreamInfo

	// Read metadata blocks.
	isFirst := true
	var isLast bool
	for !isLast {
		// Read metadata block header.
		block, err := meta.NewBlock(s.r)
		if err != nil {
			return err
		}
		if block.Header.IsLast {
			isLast = true
		}

		// The first block type must be StreamInfo.
		if isFirst {
			if block.Header.BlockType != meta.TypeStreamInfo {
				return fmt.Errorf("flac.NewStream: first block type is invalid; expected %d (StreamInfo), got %d", meta.TypeStreamInfo, block.Header.BlockType)
			}
			isFirst = false
		}

		// Check if the metadata block type is present in the provided types
		// bitfield.
		if block.Header.BlockType & types != 0 {
			// Read metadata block body.
			err = block.Parse()
			if err != nil {
				return err
			}
		} else {
			// Ignore metadata block body.
			err = block.Skip()
			if err != nil {
				return err
			}
		}

		// Store the decoded metadata block.
		s.MetaBlocks = append(s.MetaBlocks, block)
	}

	return nil
}

// ParseFrames reads and parses the audio frames of the stream.
func (s *Stream) ParseFrames() (err error) {
	// The first block is always a StreamInfo block.
	si := s.MetaBlocks[0].Body.(*meta.StreamInfo)

	// Read audio frames.
	// uint64 won't overflow since the max value of SampleCount is
	// 0x0000000FFFFFFFFF.
	var i uint64
	for i < si.SampleCount {
		f, err := frame.NewFrame(s.r)
		if err != nil {
			return err
		}
		s.Frames = append(s.Frames, f)
		i += uint64(len(f.SubFrames[0].Samples))
	}

	return nil
}
