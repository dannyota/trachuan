package observe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type sample struct {
	Time        time.Time                `json:"time"`
	Connections *int                     `json:"connections"`
	HostCPU     *float64                 `json:"host_cpu_percent"`
	Processes   map[string]processSample `json:"processes"`
	Services    map[string]serviceSample `json:"services,omitempty"`
}
type processSample struct {
	Count       int       `json:"count"`
	RSS         uint64    `json:"rss_bytes"`
	LifetimeCPU float64   `json:"lifetime_cpu_percent"`
	Restart     bool      `json:"restart"`
	Processes   []process `json:"processes"`
}
type procFlags []string

func (p *procFlags) String() string     { return strings.Join(*p, ",") }
func (p *procFlags) Set(v string) error { *p = append(*p, v); return nil }
func Sample(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sample", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var pg, out, cgroupRoot, services string
	var specs procFlags
	fs.StringVar(&pg, "pg", "", "postgres URL")
	fs.StringVar(&out, "out", "", "output JSONL")
	fs.Var(&specs, "proc", "name=regex")
	fs.StringVar(&cgroupRoot, "cgroup-root", "", "app.slice directory holding trachuan-<name>.service cgroups")
	fs.StringVar(&services, "service", "", "comma-separated service names")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || out == "" || (len(specs) == 0 && services == "") || (services == "") != (cgroupRoot == "") {
		return errors.New("invalid sample options")
	}
	var cgroups *cgroupReader
	if services != "" {
		names := strings.Split(services, ",")
		for _, name := range names {
			if name == "" || strings.ContainsAny(name, "/ ") {
				return errors.New("invalid service name")
			}
		}
		cgroups = newCgroupReader(cgroupRoot, names)
	}
	expressions := map[string]*regexp.Regexp{}
	for _, spec := range specs {
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || expressions[parts[0]] != nil {
			return errors.New("invalid process pattern")
		}
		pattern, err := regexp.Compile(parts[1])
		if err != nil {
			return errors.New("invalid process pattern")
		}
		expressions[parts[0]] = pattern
	}
	var conn *pgx.Conn
	if pg != "" {
		var err error
		conn, err = pgx.Connect(ctx, pg)
		if err != nil {
			return errors.New("connect sampler database failed")
		}
		defer func() { _ = conn.Close(context.Background()) }()
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create samples: %w", err)
	}
	defer func() { _ = f.Close() }()
	encoder := json.NewEncoder(f)
	history := newTracker()
	var oldTotal, oldIdle uint64
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		var s sample
		s.Time = time.Now().UTC()
		if conn != nil {
			var connections int
			if err = conn.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE backend_type = 'client backend'").Scan(&connections); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("sample database failed")
			}
			s.Connections = &connections
		}
		if cgroups != nil {
			if s.Services, err = cgroups.read(s.Time); err != nil {
				return err
			}
		}
		ps, total, err := readProcesses(ctx, expressions)
		if err != nil {
			return err
		}
		s.Processes = map[string]processSample{}
		for name, items := range ps {
			s.Processes[name] = summarizeProcesses(history, name, items)
		}
		if runtime.GOOS == "linux" {
			t, idle, e := procCPU()
			if e != nil {
				return e
			}
			if oldTotal > 0 && t > oldTotal {
				value := 100 * float64((t-oldTotal)-(idle-oldIdle)) / float64(t-oldTotal)
				s.HostCPU = &value
			}
			oldTotal, oldIdle = t, idle
		} else {
			value := total / float64(runtime.NumCPU())
			s.HostCPU = &value
		}
		if err = encoder.Encode(s); err != nil {
			return fmt.Errorf("write samples: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}
func procCPU() (uint64, uint64, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, errors.New("host CPU unavailable")
	}
	defer func() { _ = f.Close() }()
	scan := bufio.NewScanner(f)
	if !scan.Scan() {
		return 0, 0, errors.New("host CPU unavailable")
	}
	fields := strings.Fields(scan.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, errors.New("host CPU unavailable")
	}
	var total, idle uint64
	for i, s := range fields[1:] {
		if i >= 8 {
			break
		}
		n, e := strconv.ParseUint(s, 10, 64)
		if e != nil {
			return 0, 0, errors.New("host CPU unavailable")
		}
		total += n
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return total, idle, nil
}

func summarizeProcesses(history *tracker, name string, items []process) processSample {
	v := processSample{Count: len(items), Processes: items, Restart: history.observe(name, items)}
	for _, p := range items {
		v.RSS += p.RSS
		v.LifetimeCPU += p.LifetimeCPU
	}
	return v
}
