// package rotatelogs is a port of File-RotateLogs from Perl
// (https://metacpan.org/release/File-RotateLogs), and it allows
// you to automatically rotate output files when you write to them
// according to the filename pattern that you can specify.
package rotatelogs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	strftime "github.com/lestrrat-go/strftime"
	"github.com/pkg/errors"
)

func (c clockFn) Now() time.Time {
	return c()
}

// New creates a new RotateLogs object. A log filename pattern
// must be passed. Optional `Option` parameters may be passed
func New(p string, options ...Option) (*RotateLogs, error) {
	globPattern := p
	for _, re := range patternConversionRegexps {
		globPattern = re.ReplaceAllString(globPattern, "*")
	}

	pattern, err := strftime.New(p)
	if err != nil {
		return nil, errors.Wrap(err, `invalid strftime pattern`)
	}

	rl := &RotateLogs{}
	rl.apply(globPattern, pattern, options...)
	if rl.maxAge > 0 && rl.rotationCount > 0 {
		return nil, errors.New("options MaxAge and RotationCount cannot be both set")
	}
	if rl.maxAge == 0 && rl.rotationCount == 0 {
		// if both are 0, give maxAge a sane default
		rl.maxAge = 7 * 24 * time.Hour
	}
	return rl, nil
}

func (rl *RotateLogs) apply(globPattern string, pattern *strftime.Strftime, options ...Option) {
	var (
		rotationSize  int64
		rotationCount uint
		linkName      string
		maxAge        time.Duration
		handler       Handler
		forceNewFile  bool
		clock         Clock = Local

		rotationTime = 24 * time.Hour
	)

	for _, o := range options {
		switch o.Name() {
		case optkeyClock:
			clock, _ = o.Value().(Clock)
		case optkeyLinkName:
			linkName, _ = o.Value().(string)
		case optkeyMaxAge:
			maxAge, _ = o.Value().(time.Duration)
			if maxAge < 0 {
				maxAge = 0
			}
		case optkeyRotationTime:
			rotationTime, _ = o.Value().(time.Duration)
			if rotationTime < 0 {
				rotationTime = 0
			}
		case optkeyRotationSize:
			rotationSize, _ = o.Value().(int64)
			if rotationSize < 0 {
				rotationSize = 0
			}
		case optkeyRotationCount:
			rotationCount, _ = o.Value().(uint)
		case optkeyHandler:
			handler, _ = o.Value().(Handler)
		case optkeyForceNewFile:
			forceNewFile = true
		}
	}

	rl.clock = clock
	rl.eventHandler = handler
	rl.globPattern = globPattern
	rl.linkName = linkName
	rl.maxAge = maxAge
	rl.pattern = pattern
	rl.rotationTime = rotationTime
	rl.rotationSize = rotationSize
	rl.rotationCount = rotationCount
	rl.forceNewFile = forceNewFile
}

func (rl *RotateLogs) genFilename() string {
	now := rl.clock.Now()

	// XXX HACK: Truncate only happens in UTC semantics, apparently.
	// observed values for truncating given time with 86400 secs:
	//
	// before truncation: 2018/06/01 03:54:54 2018-06-01T03:18:00+09:00
	// after  truncation: 2018/06/01 03:54:54 2018-05-31T09:00:00+09:00
	//
	// This is really annoying when we want to truncate in local time
	// so we hack: we take the apparent local time in the local zone,
	// and pretend that it's in UTC. do our math, and put it back to
	// the local zone
	var base time.Time
	if now.Location() != time.UTC {
		base = time.Date(
			now.Year(),
			now.Month(),
			now.Day(),
			now.Hour(),
			now.Minute(),
			now.Second(),
			now.Nanosecond(),
			time.UTC,
		)
		base = base.Truncate(time.Duration(rl.rotationTime))
		base = time.Date(
			base.Year(),
			base.Month(),
			base.Day(),
			base.Hour(),
			base.Minute(),
			base.Second(),
			base.Nanosecond(),
			base.Location(),
		)
	} else {
		base = now.Truncate(time.Duration(rl.rotationTime))
	}
	return rl.pattern.FormatString(base)
}

// Write satisfies the io.Writer interface. It writes to the
// appropriate file handle that is currently being used.
// If we have reached rotation time, the target file gets
// automatically rotated, and also purged if necessary.
func (rl *RotateLogs) Write(p []byte) (n int, err error) {
	// Guard against concurrent writes
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	out, err := rl.getWriterNoLock(false, false)
	if err != nil {
		return 0, errors.Wrap(err, `failed to acquite target io.Writer`)
	}

	return out.Write(p)
}

// must be locked during this operation
func (rl *RotateLogs) getWriterNoLock(bailOnRotateFail, useGenerationalNames bool) (io.Writer, error) {
	var (
		baseFn               = rl.genFilename()
		previousFn           = rl.curFn
		filename, generation = rl.findNextFile(baseFn, useGenerationalNames)
	)
	if len(filename) == 0 {
		return rl.outFh, nil
	}

	// make sure the dir is existed, eg:
	// ./foo/bar/baz/hello.log must make sure ./foo/bar/baz is existed
	dirname := filepath.Dir(filename)
	if err := os.MkdirAll(dirname, 0755); err != nil {
		return nil, errors.Wrapf(err, "failed to create directory %s", dirname)
	}
	// if we got here, then we need to create a file
	fh, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errors.Errorf("failed to open file %s: %s", rl.pattern, err)
	}

	if err := rl.rotateNoLock(filename); err != nil {
		err = errors.Wrap(err, "failed to rotate")
		if bailOnRotateFail {
			// Failure to rotate is a problem, but it's really not a great
			// idea to stop your application just because you couldn't rename
			// your log.
			//
			// We only return this error when explicitly needed (as specified by bailOnRotateFail)
			//
			// However, we *NEED* to close `fh` here
			if fh != nil { // probably can't happen, but being paranoid
				fh.Close()
			}
			return nil, err
		}
	}

	rl.outFh.Close()
	rl.outFh = fh
	rl.curBaseFn = baseFn
	rl.curFn = filename
	rl.generation = generation

	if h := rl.eventHandler; h != nil {
		go h.Handle(&FileRotatedEvent{
			prev:    previousFn,
			current: filename,
		})
	}
	return fh, nil
}

func (rl *RotateLogs) findNextFile(baseFn string, useGenerationalNames bool) (string, int) {
	var (
		forceNewFile bool
		generation   = rl.generation
		filename     = baseFn
		sizeRotation = false
	)

	fi, err := os.Stat(rl.curFn)
	if err == nil && rl.rotationSize > 0 && rl.rotationSize <= fi.Size() {
		forceNewFile = true
		sizeRotation = true
	}

	if baseFn != rl.curBaseFn {
		generation = 0
		// even though this is the first write after calling New(),
		// check if a new file needs to be created
		if rl.forceNewFile {
			forceNewFile = true
		}
	} else {
		if !useGenerationalNames && !sizeRotation {
			// nothing to do
			return "", -1
		}
		forceNewFile = true
		generation++
	}
	if forceNewFile {
		// A new file has been requested. Instead of just using the
		// regular strftime pattern, we create a new file name using
		// generational names such as "foo.1", "foo.2", "foo.3", etc
		var name string
		for {
			if generation == 0 {
				name = filename
			} else {
				name = fmt.Sprintf("%s.%d", filename, generation)
			}
			if _, err := os.Stat(name); err != nil {
				filename = name
				break
			}
			generation++
		}
	}
	return filename, generation
}

// CurrentFileName returns the current file name that
// the RotateLogs object is writing to
func (rl *RotateLogs) CurrentFileName() string {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()
	return rl.curFn
}

var patternConversionRegexps = []*regexp.Regexp{
	regexp.MustCompile(`%[%+A-Za-z]`),
	regexp.MustCompile(`\*+`),
}

type cleanupGuard struct {
	enable bool
	fn     func()
	mutex  sync.Mutex
}

func (g *cleanupGuard) Enable() {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.enable = true
}
func (g *cleanupGuard) Run() {
	g.fn()
}

// Rotate forcefully rotates the log files. If the generated file name
// clash because file already exists, a numeric suffix of the form
// ".1", ".2", ".3" and so forth are appended to the end of the log file
//
// Thie method can be used in conjunction with a signal handler so to
// emulate servers that generate new log files when they receive a
// SIGHUP
func (rl *RotateLogs) Rotate() error {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	if _, err := rl.getWriterNoLock(true, true); err != nil {
		return err
	}
	return nil
}

func (rl *RotateLogs) rotateNoLock(filename string) error {
	lockfn := filename + `_lock`
	fh, err := os.OpenFile(lockfn, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		// Can't lock, just return
		return err
	}

	var guard = cleanupGuard{
		fn: func() {
			fh.Close()
			os.Remove(lockfn)
		},
	}
	defer guard.Run()

	if err = rl.linkFile(filename); err != nil {
		return err
	}
	if rl.maxAge <= 0 && rl.rotationCount <= 0 {
		return errors.New("panic: maxAge and rotationCount are both set")
	}

	matches, err := filepath.Glob(rl.globPattern)
	if err != nil {
		return err
	}

	cutoff := rl.clock.Now().Add(-1 * rl.maxAge)
	toUnlink := rl.getUnLinkFiles(matches, cutoff)
	if len(toUnlink) <= 0 {
		return nil
	}
	guard.Enable()
	go func() {
		// unlink files on a separate goroutine
		for _, path := range toUnlink {
			os.Remove(path)
		}
	}()

	return nil
}

func (rl *RotateLogs) linkFile(filename string) error {
	if rl.linkName != "" {
		tmpLinkName := filename + `_symlink`

		// Change how the link name is generated based on where the
		// target location is. if the location is directly underneath
		// the main filename's parent directory, then we create a
		// symlink with a relative path
		var (
			linkDest = filename
			linkDir  = filepath.Dir(rl.linkName)
			baseDir  = filepath.Dir(filename)
		)
		if strings.Contains(rl.linkName, baseDir) {
			tmp, err := filepath.Rel(linkDir, filename)
			if err != nil {
				return errors.Wrapf(err, `failed to evaluate relative path from %#v to %#v`, baseDir, rl.linkName)
			}
			linkDest = tmp
		}
		if err := os.Symlink(linkDest, tmpLinkName); err != nil {
			return errors.Wrap(err, `failed to create new symlink`)
		}
		// the directory where rl.linkName should be created must exist
		if _, err := os.Stat(linkDir); err != nil { // Assume err != nil means the directory doesn't exist
			if err := os.MkdirAll(linkDir, 0755); err != nil {
				return errors.Wrapf(err, `failed to create directory %s`, linkDir)
			}
		}
		if err := os.Rename(tmpLinkName, rl.linkName); err != nil {
			return errors.Wrap(err, `failed to rename new symlink`)
		}
	}
	return nil
}

func (rl *RotateLogs) getUnLinkFiles(matches []string, cutoff time.Time) []string {
	var toUnlink []string
	for _, path := range matches {
		// Ignore lock files
		if strings.HasSuffix(path, "_lock") || strings.HasSuffix(path, "_symlink") {
			continue
		}

		fi, err := os.Stat(path)
		if err != nil {
			continue
		}

		fl, err := os.Lstat(path)
		if err != nil {
			continue
		}

		if rl.maxAge > 0 && fi.ModTime().After(cutoff) {
			continue
		}

		if rl.rotationCount > 0 && fl.Mode()&os.ModeSymlink == os.ModeSymlink {
			continue
		}
		toUnlink = append(toUnlink, path)
	}
	if rl.rotationCount > 0 {
		// Only delete if we have more than rotationCount
		if rl.rotationCount >= uint(len(toUnlink)) {
			return nil
		}

		toUnlink = toUnlink[:len(toUnlink)-int(rl.rotationCount)]
	}
	return toUnlink
}

// Close satisfies the io.Closer interface. You must
// call this method if you performed any writes to
// the object.
func (rl *RotateLogs) Close() error {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	if rl.outFh == nil {
		return nil
	}

	rl.outFh.Close()
	rl.outFh = nil
	return nil
}
