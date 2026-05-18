package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
)

type profileSession struct {
	cpuPath string
	cpuFile *os.File
	memPath string
	memFile *os.File
}

func startProfiles(flags Flags) (*profileSession, error) {
	session := &profileSession{}
	if flags.CPUProf != "" {
		f, err := os.Create(flags.CPUProf)
		if err != nil {
			return nil, fmt.Errorf("create CPU profile %q: %w", flags.CPUProf, err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("start CPU profile %q: %w", flags.CPUProf, err)
		}
		session.cpuPath = flags.CPUProf
		session.cpuFile = f
	}
	if flags.MemProf != "" {
		f, err := os.Create(flags.MemProf)
		if err != nil {
			_ = session.Close()
			return nil, fmt.Errorf("create memory profile %q: %w", flags.MemProf, err)
		}
		session.memPath = flags.MemProf
		session.memFile = f
	}
	return session, nil
}

func (s *profileSession) Close() error {
	if s == nil {
		return nil
	}
	var err error
	if s.cpuFile != nil {
		pprof.StopCPUProfile()
		err = errors.Join(err, closeProfileFile("CPU", s.cpuPath, s.cpuFile))
		s.cpuFile = nil
	}
	if s.memFile != nil {
		runtime.GC()
		if writeErr := pprof.WriteHeapProfile(s.memFile); writeErr != nil {
			err = errors.Join(err, fmt.Errorf("write memory profile %q: %w", s.memPath, writeErr))
		}
		err = errors.Join(err, closeProfileFile("memory", s.memPath, s.memFile))
		s.memFile = nil
	}
	return err
}

func closeProfileFile(kind, path string, f *os.File) error {
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s profile %q: %w", kind, path, err)
	}
	return nil
}
