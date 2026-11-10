package worker

import (
	"fmt"
	"net/url"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/config"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func sourceClients(cfg config.Config, gate sourcegate.Gate) (*vbpl.Client, *chinhphu.Client, error) {
	vbOptions := []vbpl.Option{vbpl.WithLimits(cfg.VBPLRate, cfg.VBPLInFlight), vbpl.WithFileInFlight(cfg.VBPLFileInFlight), vbpl.WithFileHosts(strings.Split(cfg.VBPLFileHosts, ",")...), vbpl.WithGate(gate)}
	if cfg.VBPLLoopbackHTTP {
		vbOptions = append(vbOptions, vbpl.WithTestHTTP())
	}
	cpOptions := []chinhphu.Option{chinhphu.WithGate(gate), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: cfg.ChinhphuRate, Burst: 1, InFlight: cfg.ChinhphuInFlight, Redirects: 3}), chinhphu.WithUserAgent(workerVersion())}
	if cfg.ChinhphuLoopbackHTTP {
		for _, raw := range []string{cfg.ChinhphuURL, cfg.ChinhphuFileURL} {
			u, err := url.Parse(raw)
			if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
				return nil, nil, fmt.Errorf("invalid chinhphu loopback URL")
			}
		}
		cpOptions = append(cpOptions, chinhphu.WithBaseURLs(cfg.ChinhphuURL, cfg.ChinhphuFileURL))
	} else if cfg.ChinhphuURL != "https://vanban.chinhphu.vn" || cfg.ChinhphuFileURL != "https://datafiles.chinhphu.vn" {
		return nil, nil, fmt.Errorf("invalid chinhphu source URLs")
	}
	return vbpl.NewWithBase(cfg.VBPLURL, workerVersion(), vbOptions...), chinhphu.New(cpOptions...), nil
}
