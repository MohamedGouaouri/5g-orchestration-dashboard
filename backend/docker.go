package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Docker is a minimal Docker Engine API client over the unix socket.
type Docker struct {
	hc *http.Client
}

func NewDocker(sock string) *Docker {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
		MaxIdleConns: 10,
	}
	return &Docker{hc: &http.Client{Transport: tr, Timeout: 15 * time.Second}}
}

func (d *Docker) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("docker %s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (d *Docker) getJSON(ctx context.Context, path string, out any) error {
	resp, err := d.do(ctx, "GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

type ContainerSummary struct {
	ID     string   `json:"Id"`
	Names  []string `json:"Names"`
	Image  string   `json:"Image"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

func (c ContainerSummary) Name() string {
	if len(c.Names) == 0 {
		return c.ID[:12]
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

func (d *Docker) List(ctx context.Context) ([]ContainerSummary, error) {
	var out []ContainerSummary
	err := d.getJSON(ctx, "/containers/json?all=1", &out)
	return out, err
}

type ContainerInspect struct {
	State struct {
		Status    string `json:"Status"`
		Running   bool   `json:"Running"`
		StartedAt string `json:"StartedAt"`
		Health    *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	Config struct {
		Tty bool `json:"Tty"`
	} `json:"Config"`
}

func (d *Docker) Inspect(ctx context.Context, name string) (*ContainerInspect, error) {
	var out ContainerInspect
	err := d.getJSON(ctx, "/containers/"+url.PathEscape(name)+"/json", &out)
	return &out, err
}

type statsResp struct {
	CPUStats    cpuStats `json:"cpu_stats"`
	PreCPUStats cpuStats `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}
type cpuStats struct {
	CPUUsage struct {
		TotalUsage uint64 `json:"total_usage"`
	} `json:"cpu_usage"`
	SystemCPUUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs     int    `json:"online_cpus"`
}

// Stats returns CPU% and memory (MB) for a running container.
func (d *Docker) Stats(ctx context.Context, name string) (float64, float64, error) {
	var s statsResp
	if err := d.getJSON(ctx, "/containers/"+url.PathEscape(name)+"/stats?stream=false", &s); err != nil {
		return 0, 0, err
	}
	cpu := 0.0
	cd := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sd := float64(s.CPUStats.SystemCPUUsage) - float64(s.PreCPUStats.SystemCPUUsage)
	if sd > 0 && cd > 0 {
		n := s.CPUStats.OnlineCPUs
		if n == 0 {
			n = 1
		}
		cpu = cd / sd * float64(n) * 100
	}
	mem := float64(s.MemoryStats.Usage)
	if v, ok := s.MemoryStats.Stats["inactive_file"]; ok && v < s.MemoryStats.Usage {
		mem -= float64(v)
	}
	return cpu, mem / 1024 / 1024, nil
}

// demux reads a docker multiplexed stream (non-TTY) and returns concatenated payload.
func demux(r io.Reader, tty bool) ([]byte, error) {
	if tty {
		return io.ReadAll(r)
	}
	var out bytes.Buffer
	hdr := make([]byte, 8)
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		if _, err := io.ReadFull(br, hdr); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return out.Bytes(), nil
			}
			return out.Bytes(), err
		}
		n := binary.BigEndian.Uint32(hdr[4:])
		if _, err := io.CopyN(&out, br, int64(n)); err != nil {
			return out.Bytes(), err
		}
	}
}

// Logs returns log text. since is unix seconds (0 = beginning); tail <=0 means all.
func (d *Docker) Logs(ctx context.Context, name string, since int64, tail int) (string, error) {
	q := url.Values{}
	q.Set("stdout", "1")
	q.Set("stderr", "1")
	q.Set("timestamps", "1")
	if since > 0 {
		q.Set("since", strconv.FormatInt(since, 10))
	}
	if tail > 0 {
		q.Set("tail", strconv.Itoa(tail))
	} else {
		q.Set("tail", "all")
	}
	ins, err := d.Inspect(ctx, name)
	if err != nil {
		return "", err
	}
	resp, err := d.do(ctx, "GET", "/containers/"+url.PathEscape(name)+"/logs?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := demux(resp.Body, ins.Config.Tty)
	return string(b), err
}

// Exec runs a command in a container and returns stdout+stderr.
func (d *Docker) Exec(ctx context.Context, name string, cmd ...string) (string, error) {
	resp, err := d.do(ctx, "POST", "/containers/"+url.PathEscape(name)+"/exec", map[string]any{
		"AttachStdout": true, "AttachStderr": true, "Tty": false, "Cmd": cmd,
	})
	if err != nil {
		return "", err
	}
	var created struct {
		ID string `json:"Id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	resp, err = d.do(ctx, "POST", "/exec/"+created.ID+"/start", map[string]any{"Detach": false, "Tty": false})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := demux(resp.Body, false)
	return string(b), err
}

func (d *Docker) Restart(ctx context.Context, name string) error {
	resp, err := d.do(ctx, "POST", "/containers/"+url.PathEscape(name)+"/restart?t=10", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
