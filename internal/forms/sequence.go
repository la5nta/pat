package forms

import (
	"encoding/json"
	"io"
	"os"
)

type Sequence struct {
	f   *os.File
	err error
}

func OpenSequence(path string) Sequence {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	return Sequence{f, err}
}

func (s Sequence) Close() error {
	if s.err != nil {
		return s.err
	}
	return s.f.Close()
}

func (s Sequence) Load() (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	var seq int64
	if err := json.NewDecoder(s.f).Decode(&seq); err != nil {
		if err != io.EOF {
			return 0, err
		}
	}
	return seq, nil
}

func (s Sequence) Set(seq int64) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	if err := s.f.Truncate(0); err != nil {
		return 0, err
	}
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	defer s.f.Sync()
	return seq, json.NewEncoder(s.f).Encode(&seq)
}

func (s Sequence) Incr(increment int64) (int64, error) {
	seq, err := s.Load()
	if err != nil {
		return 0, err
	}
	return s.Set(seq + increment)
}
