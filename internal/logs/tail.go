package logs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/papertrail/go-tail/follower"
)

type Tailer struct {
	logger       *slog.Logger
	rotatedMatch *regexp.Regexp
}

func NewTailer(logger *slog.Logger, logFilename string, deleteRotated bool) *Tailer {
	t := &Tailer{logger: logger}
	if !deleteRotated {
		return t
	}

	matcher, err := rotatedLogMatcher(logFilename)
	if err != nil {
		logger.Error("could not compile log_filename matcher; rotated-file deletion disabled",
			"log_filename", logFilename, "error", err)

		return t
	}
	t.rotatedMatch = matcher

	return t
}

func (t Tailer) Tail(ctx context.Context, logDir string) (<-chan []byte, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(logDir); err != nil {
		closeErr := watcher.Close()
		return nil, errors.Join(err, closeErr)
	}

	out := make(chan []byte)
	s := &session{out: out, logger: t.logger, dir: logDir, rotatedMatch: t.rotatedMatch}

	if path, ok := s.newest(); ok {
		if err := s.open(path, io.SeekEnd); err != nil {
			t.logger.Error("could not tail log file", "path", path, "error", err)
		}
	}

	go s.run(ctx, watcher)
	return out, nil
}

// session holds the one file currently being followed for a single Tail call.
type session struct {
	out    chan<- []byte
	logger *slog.Logger

	// dir is the watched directory. The session always follows the newest .json file within it.
	dir string

	rotatedMatch *regexp.Regexp

	cur     *follower.Follower
	curPath string
	curMod  time.Time
}

// run forwards lines from the current file and, on watcher events, switches the file to follow.
func (s *session) run(ctx context.Context, watcher *fsnotify.Watcher) {
	defer close(s.out)
	defer s.closeCurrent()
	defer watcher.Close()

	for {
		var lines <-chan follower.Line
		if s.cur != nil {
			lines = s.cur.Lines()
		}

		select {
		case <-ctx.Done():
			return

		case line, ok := <-lines:
			if !ok {
				s.currentDied()
				continue
			}
			select {
			case s.out <- line.Bytes():
			case <-ctx.Done():
				return
			}

		case event := <-watcher.Events:
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
				s.switchIfNewer(event.Name)
			}

		case err := <-watcher.Errors:
			s.logger.Error("fsnotify watcher failure", "error", err)
		}
	}
}

func (s *session) currentDied() {
	if s.cur == nil {
		return
	}
	if err := s.cur.Err(); err != nil {
		s.logger.Error("log file tail failed", "path", s.curPath, "error", err)
	}
	s.cur, s.curPath = nil, ""
}

func (s *session) switchIfNewer(path string) {
	if path == s.curPath || !isJSONLog(path) {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}
	if s.curPath != "" && !info.ModTime().After(s.curMod) {
		return
	}

	rotated := s.curPath != ""
	if err := s.open(path, io.SeekEnd); err != nil {
		s.logger.Error("could not tail log file", "path", path, "error", err)

		return
	}
	if rotated {
		s.deleteOlderRotatedFiles()
	}
}

func (s *session) deleteOlderRotatedFiles() {
	if s.rotatedMatch == nil {
		return
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		s.logger.Error("could not list log directory for cleanup", "dir", s.dir, "error", err)

		return
	}

	curBase := filepath.Base(s.curPath)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == curBase || !s.rotatedMatch.MatchString(name) {
			continue
		}
		info, infoErr := e.Info()
		if infoErr != nil || !info.Mode().IsRegular() || !info.ModTime().Before(s.curMod) {
			continue
		}

		path := filepath.Join(s.dir, name)
		if rmErr := os.Remove(path); rmErr != nil {
			s.logger.Error("could not delete rotated log file", "path", path, "error", rmErr)
		} else {
			s.logger.Info("deleted rotated log file", "path", path)
		}
	}
}

// open starts following path and closes the previous file.
func (s *session) open(path string, whence int) error {
	f, err := follower.New(path, follower.Config{Whence: whence, Reopen: true})
	if err != nil {
		return err
	}

	s.closeCurrent()
	s.cur, s.curPath = f, path
	if info, err := os.Stat(path); err == nil {
		s.curMod = info.ModTime()
	}
	s.logger.Debug("tailing log file", "path", path)
	return nil
}

func (s *session) closeCurrent() {
	if s.cur == nil {
		return
	}
	s.cur.Close()
	s.cur = nil
}

func (s *session) newest() (string, bool) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return "", false
	}

	var best string
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !isJSONLog(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = filepath.Join(s.dir, e.Name()), info.ModTime()
		}
	}
	return best, best != ""
}

func isJSONLog(name string) bool {
	return filepath.Ext(name) == ".json"
}
