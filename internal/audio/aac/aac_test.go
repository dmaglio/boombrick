package aac

import "testing"

func TestRejectsMissingConfig(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) should fail")
	}
}

func TestOpensLCStereo(t *testing.T) {
	// AAC-LC (object type 2), 44.1 kHz (frequency index 4), two channels.
	d, err := New([]byte{0x12, 0x10})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Rate() != 44100 {
		t.Fatalf("Rate = %d, want 44100", d.Rate())
	}
}

func TestUpsamplesImplicitSBR(t *testing.T) {
	// AAC-LC, 22.05 kHz, mono, with no SBR signalling: faad2 assumes SBR.
	d, err := New([]byte{0x13, 0x88})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Rate() != 44100 {
		t.Fatalf("Rate = %d, want 44100", d.Rate())
	}
}

func TestRejectsBadConfig(t *testing.T) {
	// Object type 0 doesn't exist; channel configuration 15 is reserved.
	for _, config := range [][]byte{{0x00, 0x00}, {0x12, 0x78}} {
		if d, err := New(config); err == nil {
			d.Close()
			t.Fatalf("New(%x) should fail", config)
		}
	}
}
