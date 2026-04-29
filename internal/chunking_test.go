package internal

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestSplitIntoChunks_BelowThreshold(t *testing.T) {
	// 100KB frame — already below the threshold but splitIntoChunks tolerates
	// it by emitting a single chunk so callers can rely on a non-empty slice.
	payload := bytes.Repeat([]byte{0xAB}, 100*1024)
	chunks, err := splitIntoChunks("req-1", payload)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) != 2 {
		// 100 KB / 64 KB → 2 chunks.
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestSplitIntoChunks_LargeFrame(t *testing.T) {
	src := make([]byte, 5*1024*1024+128) // 5 MB + tail
	if _, err := rand.Read(src); err != nil {
		t.Fatal(err)
	}
	chunks, err := splitIntoChunks("req-roundtrip", src)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	expected := (len(src) + chunkSize - 1) / chunkSize
	if len(chunks) != expected {
		t.Fatalf("expected %d chunks, got %d", expected, len(chunks))
	}

	// Decode each chunk frame and feed an assembler. The recombined bytes
	// must round-trip the source frame exactly.
	asm := newChunkAssembler()
	var assembled []byte
	for i, c := range chunks {
		frame, err := decodeBinaryFrame(c)
		if err != nil {
			t.Fatalf("chunk %d decode: %v", i, err)
		}
		if frame.msgType != msgTypeChunk {
			t.Fatalf("chunk %d msgType = 0x%02x", i, frame.msgType)
		}
		meta, err := extractChunkMeta(frame)
		if err != nil {
			t.Fatalf("chunk %d meta: %v", i, err)
		}
		if meta.RequestID != "req-roundtrip" {
			t.Fatalf("chunk %d requestId = %q", i, meta.RequestID)
		}
		if meta.Total != expected {
			t.Fatalf("chunk %d total = %d", i, meta.Total)
		}
		out, err := asm.receive(meta, frame.payload)
		if err != nil {
			t.Fatalf("chunk %d receive: %v", i, err)
		}
		if i < expected-1 {
			if out != nil {
				t.Fatalf("chunk %d returned full frame early", i)
			}
			continue
		}
		if out == nil {
			t.Fatalf("last chunk did not return assembled bytes")
		}
		assembled = out
	}
	if !bytes.Equal(src, assembled) {
		t.Fatalf("assembled bytes differ from source")
	}
	if asm.pending() != 0 {
		t.Fatalf("assembler still has pending streams: %d", asm.pending())
	}
}

func TestChunkAssembler_OutOfOrder(t *testing.T) {
	src := bytes.Repeat([]byte{0x42}, chunkSize*3)
	chunks, err := splitIntoChunks("req-ooo", src)
	if err != nil {
		t.Fatalf("split: %v", err)
	}

	// Feed chunks in reverse — assembler must wait until all arrive.
	asm := newChunkAssembler()
	var assembled []byte
	for i := len(chunks) - 1; i >= 0; i-- {
		frame, err := decodeBinaryFrame(chunks[i])
		if err != nil {
			t.Fatal(err)
		}
		meta, err := extractChunkMeta(frame)
		if err != nil {
			t.Fatal(err)
		}
		out, err := asm.receive(meta, frame.payload)
		if err != nil {
			t.Fatalf("recv %d: %v", meta.Seq, err)
		}
		if i > 0 && out != nil {
			t.Fatalf("recv %d returned full frame too early", meta.Seq)
		}
		if i == 0 {
			if out == nil {
				t.Fatal("final chunk did not yield assembled frame")
			}
			assembled = out
		}
	}
	if !bytes.Equal(src, assembled) {
		t.Fatal("assembled bytes mismatch")
	}
}

func TestChunkAssembler_DuplicateSeq(t *testing.T) {
	src := bytes.Repeat([]byte{0x55}, chunkSize*2)
	chunks, err := splitIntoChunks("req-dup", src)
	if err != nil {
		t.Fatal(err)
	}

	asm := newChunkAssembler()
	frame, _ := decodeBinaryFrame(chunks[0])
	meta, _ := extractChunkMeta(frame)
	if _, err := asm.receive(meta, frame.payload); err != nil {
		t.Fatal(err)
	}
	if _, err := asm.receive(meta, frame.payload); err == nil {
		t.Fatal("expected error on duplicate seq")
	}
}

func TestChunkAssembler_TotalMismatch(t *testing.T) {
	asm := newChunkAssembler()
	if _, err := asm.receive(chunkMeta{RequestID: "x", Seq: 0, Total: 2}, []byte("ab")); err != nil {
		t.Fatal(err)
	}
	// Conflicting total on a second arrival under the same requestId — this
	// is impossible in the wire protocol but the assembler must reject it
	// rather than silently corrupt the buffer.
	if _, err := asm.receive(chunkMeta{RequestID: "x", Seq: 1, Total: 3}, []byte("c")); err == nil {
		t.Fatal("expected total mismatch error")
	}
}

func TestShouldChunk_Threshold(t *testing.T) {
	if shouldChunk(make([]byte, chunkThresholdSize)) {
		t.Fatalf("frame at exactly the threshold should not chunk")
	}
	if !shouldChunk(make([]byte, chunkThresholdSize+1)) {
		t.Fatalf("frame above threshold should chunk")
	}
}
