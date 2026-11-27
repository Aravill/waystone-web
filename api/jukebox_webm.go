package api

import (
	"encoding/binary"
	"errors"
)

// webmReader splits a live WebM stream (MediaRecorder output, arriving in arbitrary chunks) into an init
// segment and self-contained known-size cluster frames, so a listener can join at any frame boundary.
// It does no I/O: Feed takes bytes and returns what became complete.

const (
	webmMaxBuf     = 1 << 20
	webmMaxElement = 256 << 10
)

const (
	webmIDEBML     = 0x1A45DFA3
	webmIDSegment  = 0x18538067
	webmIDInfo     = 0x1549A966
	webmIDTracks   = 0x1654AE6B
	webmIDSeekHead = 0x114D9B74
	webmIDCues     = 0x1C53BB6B
	webmIDTags     = 0x1254C367
	webmIDCluster  = 0x1F43B675
	webmIDTimecode = 0xE7
	webmIDSimple   = 0xA3
	webmIDGroup    = 0xA0
)

const (
	webmPhaseHeader = iota
	webmPhaseSegment
	webmPhaseTop // before the first cluster: Segment children
	webmPhaseClusterHead
	webmPhaseCluster
)

type webmReader struct {
	buf         []byte
	phase       int
	init        []byte
	initDone    bool
	clusterLeft int64 // bytes of the current cluster still to read, -1 when its size is unknown
	tc          uint64
	blocks      []byte
}

var errWebm = errors.New("invalid webm stream")

// vintID reads an element ID (marker bits kept). ok is false when more bytes are needed.
func vintID(b []byte) (id uint32, n int, ok bool, err error) {
	if len(b) == 0 {
		return 0, 0, false, nil
	}
	first := b[0]
	n = 1
	for mask := byte(0x80); n <= 4 && first&mask == 0; mask >>= 1 {
		n++
	}
	if n > 4 {
		return 0, 0, false, errWebm
	}
	if len(b) < n {
		return 0, 0, false, nil
	}
	for i := 0; i < n; i++ {
		id = id<<8 | uint32(b[i])
	}
	return id, n, true, nil
}

// vintSize reads an element size (marker removed); unknown is true when every value bit is 1.
func vintSize(b []byte) (size int64, n int, unknown bool, ok bool, err error) {
	if len(b) == 0 {
		return 0, 0, false, false, nil
	}
	first := b[0]
	n = 1
	for mask := byte(0x80); n <= 8 && first&mask == 0; mask >>= 1 {
		n++
	}
	if n > 8 {
		return 0, 0, false, false, errWebm
	}
	if len(b) < n {
		return 0, 0, false, false, nil
	}
	v := uint64(first) & (0xFF >> uint(n))
	allOnes := v == uint64(0xFF>>uint(n))
	for i := 1; i < n; i++ {
		v = v<<8 | uint64(b[i])
		if b[i] != 0xFF {
			allOnes = false
		}
	}
	if allOnes {
		return 0, n, true, true, nil
	}
	if v > 1<<40 {
		return 0, 0, false, false, errWebm
	}
	return int64(v), n, false, true, nil
}

func webmSegmentLevel(id uint32) bool {
	switch id {
	case webmIDCluster, webmIDCues, webmIDTags, webmIDSeekHead, webmIDInfo, webmIDTracks:
		return true
	}
	return false
}

// Feed appends one chunk. It returns the init segment once (when complete) and any complete cluster frames.
func (r *webmReader) Feed(p []byte) (init []byte, frames [][]byte, err error) {
	r.buf = append(r.buf, p...)
	if len(r.buf) > webmMaxBuf {
		return nil, nil, errWebm
	}
	for {
		progressed, ferr := r.step(&init, &frames)
		if ferr != nil {
			return nil, nil, ferr
		}
		if !progressed {
			break
		}
	}
	r.flush(&frames)
	return init, frames, nil
}

func (r *webmReader) flush(frames *[][]byte) {
	if len(r.blocks) == 0 {
		return
	}
	n := uint64(10 + len(r.blocks))
	f := make([]byte, 0, 4+8+10+len(r.blocks))
	f = append(f, 0x1F, 0x43, 0xB6, 0x75, 0x01)
	var nb [8]byte
	binary.BigEndian.PutUint64(nb[:], n)
	f = append(f, nb[1:]...)
	f = append(f, 0xE7, 0x88)
	var tb [8]byte
	binary.BigEndian.PutUint64(tb[:], r.tc)
	f = append(f, tb[:]...)
	f = append(f, r.blocks...)
	*frames = append(*frames, f)
	r.blocks = nil
}

func (r *webmReader) consume(n int) {
	r.buf = r.buf[n:]
	if r.clusterLeft > 0 {
		r.clusterLeft -= int64(n)
		if r.clusterLeft < 0 {
			r.clusterLeft = 0
		}
	}
}

// step handles one element; progressed is false when it needs more bytes.
func (r *webmReader) step(init *[]byte, frames *[][]byte) (progressed bool, err error) {
	id, idLen, ok, err := vintID(r.buf)
	if err != nil || !ok {
		return false, err
	}
	size, szLen, unknown, ok, err := vintSize(r.buf[idLen:])
	if err != nil || !ok {
		return false, err
	}
	hdr := idLen + szLen

	switch r.phase {
	case webmPhaseHeader:
		if id != webmIDEBML || unknown {
			return false, errWebm
		}
		if size > webmMaxElement {
			return false, errWebm
		}
		total := hdr + int(size)
		if len(r.buf) < total {
			return false, nil
		}
		r.init = append(r.init, r.buf[:total]...)
		r.init = append(r.init, 0x18, 0x53, 0x80, 0x67, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF)
		r.buf = r.buf[total:]
		r.phase = webmPhaseSegment
		return true, nil

	case webmPhaseSegment:
		if id != webmIDSegment {
			return false, errWebm
		}
		r.buf = r.buf[hdr:] // children follow directly, known or unknown size
		r.phase = webmPhaseTop
		return true, nil

	case webmPhaseTop:
		if id == webmIDCluster {
			if !r.initDone {
				r.initDone = true
				*init = r.init
			}
			r.phase = webmPhaseClusterHead
			return true, nil
		}
		if unknown || size > webmMaxElement {
			return false, errWebm
		}
		total := hdr + int(size)
		if len(r.buf) < total {
			return false, nil
		}
		if !r.initDone && (id == webmIDInfo || id == webmIDTracks) {
			r.init = append(r.init, r.buf[:total]...)
		}
		r.buf = r.buf[total:]
		return true, nil

	case webmPhaseClusterHead:
		// Only reached right at a Cluster ID.
		r.flush(frames)
		if unknown {
			r.clusterLeft = -1
		} else {
			r.clusterLeft = size
		}
		r.tc = 0
		r.buf = r.buf[hdr:]
		r.phase = webmPhaseCluster
		if r.clusterLeft == 0 {
			r.phase = webmPhaseTop
		}
		return true, nil
	}

	// webmPhaseCluster
	if r.clusterLeft < 0 && webmSegmentLevel(id) {
		// An unknown-size cluster ends at the next segment-level element.
		r.phase = webmPhaseTop
		if id == webmIDCluster {
			r.phase = webmPhaseClusterHead
			return true, nil
		}
		return r.skipTop(hdr, size, unknown)
	}
	if unknown || size > webmMaxElement {
		return false, errWebm
	}
	total := hdr + int(size)
	if len(r.buf) < total {
		return false, nil
	}
	switch id {
	case webmIDTimecode:
		if size > 8 {
			return false, errWebm
		}
		// A new timecode inside a cluster starts a new frame with that time.
		r.flush(frames)
		var v uint64
		for _, b := range r.buf[hdr:total] {
			v = v<<8 | uint64(b)
		}
		r.tc = v
	case webmIDSimple, webmIDGroup:
		r.blocks = append(r.blocks, r.buf[:total]...)
	}
	r.consume(total)
	if r.clusterLeft == 0 {
		r.phase = webmPhaseTop
	}
	return true, nil
}

// skipTop skips a non-cluster segment-level element met after the init segment.
func (r *webmReader) skipTop(hdr int, size int64, unknown bool) (bool, error) {
	if unknown || size > webmMaxElement {
		return false, errWebm
	}
	total := hdr + int(size)
	if len(r.buf) < total {
		return false, nil
	}
	r.buf = r.buf[total:]
	return true, nil
}
