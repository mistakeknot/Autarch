package file

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// deterministicData returns a repeatable byte slice of the given size.
func deterministicData(size int) []byte {
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte(i % 251) // prime mod avoids alignment patterns
	}
	return buf
}

func benchAtomicWrite(b *testing.B, size int) {
	b.Helper()
	data := deterministicData(size)
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.dat")

	b.SetBytes(int64(size))
	b.ResetTimer()
	for b.Loop() {
		if err := AtomicWriteFile(path, data, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	// Verify correctness on final write.
	got, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		b.Fatal("data mismatch after benchmark")
	}
}

func BenchmarkAtomicWriteFile1K(b *testing.B)   { benchAtomicWrite(b, 1<<10) }
func BenchmarkAtomicWriteFile10K(b *testing.B)  { benchAtomicWrite(b, 10<<10) }
func BenchmarkAtomicWriteFile100K(b *testing.B) { benchAtomicWrite(b, 100<<10) }
func BenchmarkAtomicWriteFile1M(b *testing.B)   { benchAtomicWrite(b, 1<<20) }

// BenchmarkRawWrite provides a baseline: write + close without fsync, rename,
// locking, or dir-sync so we can isolate the safety overhead.
func BenchmarkRawWrite1K(b *testing.B) { benchRawWrite(b, 1<<10) }

func benchRawWrite(b *testing.B, size int) {
	data := deterministicData(size)
	dir := b.TempDir()
	path := filepath.Join(dir, "raw.dat")

	b.SetBytes(int64(size))
	b.ResetTimer()
	for b.Loop() {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAtomicWriteFileParallel tests contention under concurrent writers.
func BenchmarkAtomicWriteFileParallel(b *testing.B) {
	data := deterministicData(1 << 10)
	dir := b.TempDir()

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine writes to its own file to avoid lock contention
		// on the same path (which would serialize everything).
		path := filepath.Join(dir, fmt.Sprintf("p-%p.dat", pb))
		for pb.Next() {
			if err := AtomicWriteFile(path, data, 0o644); err != nil {
				b.Fatal(err)
			}
		}
	})
}
