package scanner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

type ScanResult struct {
	Clean       bool
	Threats     []string
	ScanTimeMs  int64
	ScannerName string
	FromCache   bool
	Degraded    bool
	DegradeReason string
}

type Scanner interface {
	Name() string
	Scan(ctx context.Context, path string, ext string) (*ScanResult, error)
	Close() error
}

type ScannerConfig struct {
	ClamAVHost      string
	ClamAVPort      int
	ConnectTimeout  time.Duration
	CommandTimeout  time.Duration
	MaxFileSize     int64
	RatePerSecond   float64
	Burst           int
	FailureThreshold int
	FailureWindow   time.Duration
	DegradeOnFail   bool
}

func DefaultScannerConfig() ScannerConfig {
	return ScannerConfig{
		ClamAVHost:      "127.0.0.1",
		ClamAVPort:      3310,
		ConnectTimeout:  2 * time.Second,
		CommandTimeout:  15 * time.Second,
		MaxFileSize:     200 * 1024 * 1024,
		RatePerSecond:   10,
		Burst:           50,
		FailureThreshold: 5,
		FailureWindow:   60 * time.Second,
		DegradeOnFail:   true,
	}
}

type MockScanner struct {
	signatures []string
}

func NewMockScanner() *MockScanner {
	return &MockScanner{
		signatures: []string{
			"EICAR-STANDARD-ANTIVIRUS-TEST-FILE",
			"X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS",
			"eval(base64_decode(",
			"create_function(",
			"vbscript:",
			"javascript:",
			"data:text/html",
		},
	}
}

func (s *MockScanner) Name() string { return "mock" }

func (s *MockScanner) Scan(ctx context.Context, path string, ext string) (*ScanResult, error) {
	start := time.Now()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	lowerExt := strings.ToLower(ext)
	if isScriptExtension(lowerExt) {
		return &ScanResult{
			Clean:       false,
			Threats:     []string{"script extension blocked: " + ext},
			ScanTimeMs:  time.Since(start).Milliseconds(),
			ScannerName: s.Name(),
		}, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var threats []string
	if isDangerousMagic(f) {
		threats = append(threats, "dangerous magic byte detected")
	}
	if len(threats) == 0 {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if lineNum > 5000 {
				break
			}
			lower := strings.ToLower(scanner.Text())
			for _, sig := range s.signatures {
				if strings.Contains(lower, strings.ToLower(sig)) {
					threats = append(threats, "signature match: "+sig)
					break
				}
			}
			if len(threats) > 0 {
				break
			}
		}
	}

	return &ScanResult{
		Clean:       len(threats) == 0,
		Threats:     threats,
		ScanTimeMs:  time.Since(start).Milliseconds(),
		ScannerName: s.Name(),
	}, nil
}

func (s *MockScanner) Close() error { return nil }

func isScriptExtension(ext string) bool {
	scripts := map[string]bool{
		".exe": true, ".bat": true, ".cmd": true, ".com": true,
		".sh": true, ".ps1": true, ".vbs": true, ".js": true,
		".jse": true, ".wsf": true, ".hta": true, ".jar": true,
		".php": true, ".py": true, ".pl": true, ".rb": true,
	}
	return scripts[ext]
}

func isDangerousMagic(f *os.File) bool {
	buf := make([]byte, 8)
	n, _ := f.ReadAt(buf, 0)
	if n < 2 {
		return false
	}
	if buf[0] == 'M' && buf[1] == 'Z' {
		return true
	}
	if n >= 4 && buf[0] == 0x7f && buf[1] == 'E' && buf[2] == 'L' && buf[3] == 'F' {
		return true
	}
	if n >= 4 && buf[0] == 'R' && buf[1] == 'I' && buf[2] == 'F' && buf[3] == 'F' {
	}
	return false
}

type ClamAVScanner struct {
	cfg         ScannerConfig
	limiter     *rate.Limiter
	failureTs   []int64
	failureMu   sync.Mutex
	failureCnt  int64
	lastFailing atomic.Bool
	degraded    atomic.Bool
}

func NewClamAVScanner(cfg ScannerConfig) *ClamAVScanner {
	if cfg.RatePerSecond <= 0 {
		cfg.RatePerSecond = 10
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 50
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	return &ClamAVScanner{
		cfg:     cfg,
		limiter: rate.NewLimiter(rate.Limit(cfg.RatePerSecond), cfg.Burst),
	}
}

func (s *ClamAVScanner) Name() string { return "clamav" }

func (s *ClamAVScanner) recordFailure() {
	s.failureMu.Lock()
	defer s.failureMu.Unlock()
	now := time.Now().Unix()
	windowStart := now - int64(s.cfg.FailureWindow.Seconds())
	var recent []int64
	for _, t := range s.failureTs {
		if t >= windowStart {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	s.failureTs = recent
	atomic.StoreInt64(&s.failureCnt, int64(len(recent)))
	if len(recent) >= s.cfg.FailureThreshold {
		s.lastFailing.Store(true)
		if s.cfg.DegradeOnFail {
			s.degraded.Store(true)
		}
	}
}

func (s *ClamAVScanner) recordSuccess() {
	atomic.StoreInt64(&s.failureCnt, 0)
	s.lastFailing.Store(false)
	s.failureMu.Lock()
	s.failureTs = nil
	s.failureMu.Unlock()
	if s.degraded.Load() {
		s.failureMu.Lock()
		if len(s.failureTs) == 0 {
			s.degraded.Store(false)
		}
		s.failureMu.Unlock()
	}
}

func (s *ClamAVScanner) IsDegraded() bool { return s.degraded.Load() }

func (s *ClamAVScanner) FailureCount() int64 { return atomic.LoadInt64(&s.failureCnt) }

func (s *ClamAVScanner) Scan(ctx context.Context, path string, ext string) (*ScanResult, error) {
	start := time.Now()
	if s.degraded.Load() {
		return &ScanResult{
			Clean:        true,
			ScanTimeMs:   time.Since(start).Milliseconds(),
			ScannerName:  s.Name(),
			Degraded:     true,
			DegradeReason: "clamav circuit breaker open",
		}, nil
	}

	if err := s.limiter.Wait(ctx); err != nil {
		if s.cfg.DegradeOnFail {
			return &ScanResult{
				Clean:        true,
				ScanTimeMs:   time.Since(start).Milliseconds(),
				ScannerName:  s.Name(),
				Degraded:     true,
				DegradeReason: "rate limited: " + err.Error(),
			}, nil
		}
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > s.cfg.MaxFileSize {
		return nil, errors.New("file too large for virus scan")
	}

	scanCtx, cancel := context.WithTimeout(ctx, s.cfg.CommandTimeout)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", s.cfg.ClamAVHost, s.cfg.ClamAVPort)
	conn, err := net.DialTimeout("tcp", addr, s.cfg.ConnectTimeout)
	if err != nil {
		s.recordFailure()
		if s.cfg.DegradeOnFail {
			return &ScanResult{
				Clean:        true,
				ScanTimeMs:   time.Since(start).Milliseconds(),
				ScannerName:  s.Name(),
				Degraded:     true,
				DegradeReason: "clamav unreachable: " + err.Error(),
			}, nil
		}
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(s.cfg.CommandTimeout))

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, err = conn.Write([]byte("zINSTREAM\x00"))
	if err != nil {
		s.recordFailure()
		return nil, err
	}

	buf := make([]byte, 64*1024)
	for {
		if scanCtx.Err() != nil {
			s.recordFailure()
			return nil, scanCtx.Err()
		}
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			s.recordFailure()
			return nil, err
		}
		if n > 0 {
			sizeBuf := []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
			if _, err := conn.Write(sizeBuf); err != nil {
				s.recordFailure()
				return nil, err
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				s.recordFailure()
				return nil, err
			}
		}
		if err == io.EOF {
			term := []byte{0, 0, 0, 0}
			if _, err := conn.Write(term); err != nil {
				s.recordFailure()
				return nil, err
			}
			break
		}
	}

	resp, err := bufio.NewReader(conn).ReadString('\x00')
	if err != nil {
		s.recordFailure()
		return nil, err
	}
	resp = strings.TrimSpace(resp)
	resp = strings.TrimSuffix(resp, "\x00")

	s.recordSuccess()

	clean := strings.HasSuffix(resp, "OK") || strings.Contains(resp, " FOUND") == false
	var threats []string
	if !clean {
		threats = append(threats, strings.TrimPrefix(resp, "stream: "))
	}

	return &ScanResult{
		Clean:       clean,
		Threats:     threats,
		ScanTimeMs:  time.Since(start).Milliseconds(),
		ScannerName: s.Name(),
	}, nil
}

func (s *ClamAVScanner) Close() error {
	s.degraded.Store(false)
	return nil
}

func NewDefaultScanner() Scanner {
	host := os.Getenv("CLAMAV_HOST")
	if host != "" {
		cfg := DefaultScannerConfig()
		cfg.ClamAVHost = host
		return NewClamAVScanner(cfg)
	}
	return NewMockScanner()
}
