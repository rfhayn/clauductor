package panel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The panel is opened at http://clauductor.localhost:<port>. macOS (and every
// current browser) resolves *.localhost to loopback with no /etc/hosts entry, and
// returns ::1 first, so the panel listens on [::1]:<port> as well as 127.0.0.1.
// Both listeners serve one server and one state.
//
// The Host allow-list is EXACT: 127.0.0.1, localhost, [::1] and clauductor.localhost
// on the panel's port, plus names a config adds, each of the form <label>.localhost.
// There are no wildcards: evil.localhost is refused. A *.localhost name can never
// be an attacker's DNS name (browsers never ask DNS for it), so the list still
// defeats DNS rebinding. Host names compare case-insensitively (RFC 9110 §4.2.3).
// Every state-changing request and the WebSocket upgrade must carry an Origin equal
// to "http://" + the Host of THAT request, so a page on localhost cannot drive the
// panel at clauductor.localhost, or the reverse.

// DefaultHostName is the name the panel is opened at.
const DefaultHostName = "clauductor.localhost"

// localhostNameRe is the only shape an extra host name may have: one DNS label under
// .localhost, lower case.
var localhostNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.localhost$`)

// ValidHostName reports whether a configured extra host name is allowed.
func ValidHostName(n string) bool { return localhostNameRe.MatchString(n) }

// allowedHosts is the exact Host allow-list, lower case.
func (s *Server) allowedHosts() []string {
	p := strconv.Itoa(s.Port)
	hs := []string{"127.0.0.1:" + p, "localhost:" + p, "[::1]:" + p, DefaultHostName + ":" + p}
	for _, n := range s.HostNames {
		if ValidHostName(n) {
			hs = append(hs, n+":"+p)
		}
	}
	return hs
}

// hostAllowed checks a Host header against the allow-list, case-insensitively.
func (s *Server) hostAllowed(host string) bool {
	h := strings.ToLower(host)
	for _, a := range s.allowedHosts() {
		if h == a {
			return true
		}
	}
	return false
}

// originMatches: the Origin is exactly "http://" + this request's Host, and that
// Host is allowed. Browsers serialise Origin in lower case.
func (s *Server) originMatches(origin, host string) bool {
	return s.hostAllowed(host) && strings.ToLower(origin) == "http://"+strings.ToLower(host)
}

// ListenLoopback binds 127.0.0.1:port and [::1]:<the same port>. ln6 is nil when
// the machine has no IPv6 loopback (v6why says why); then the panel is opened at
// 127.0.0.1. A [::1] port held by another process is an error, never skipped: the
// browser would send clauductor.localhost, and the token, to it.
func ListenLoopback(port int) (ln4, ln6 net.Listener, v6why string, err error) {
	ln4, err = Listen(port)
	if err != nil {
		return nil, nil, "", err
	}
	p := ln4.Addr().(*net.TCPAddr).Port
	ln6, err = net.Listen("tcp6", net.JoinHostPort("::1", strconv.Itoa(p)))
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			ln4.Close()
			return nil, nil, "", fmt.Errorf("port %d on [::1] is already in use by another process. The browser resolves %s to ::1 first, "+
				"so it would reach that process; free the port or pass --port", p, DefaultHostName)
		}
		return ln4, nil, err.Error(), nil
	}
	if err := ensureLoopback(ln6.Addr()); err != nil {
		ln4.Close()
		ln6.Close()
		return nil, nil, "", err
	}
	return ln4, ln6, "", nil
}

// PanelHost is the host the panel is opened at: clauductor.localhost when it
// listens on both loopbacks, else 127.0.0.1.
func PanelHost(dualStack bool) string {
	if dualStack {
		return DefaultHostName
	}
	return LoopbackHost
}

// healthz asks one loopback address whether it is the panel with this PID.
func healthz(ctx context.Context, base string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(cctx, http.MethodGet, base+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return strings.TrimSpace(string(body)), nil
}

// openURLChecked returns the tokened URL to open, after checking that BOTH loopback
// addresses the browser may use for clauductor.localhost answer as this panel (the
// PID beside the marker). If nothing listens on [::1], the 127.0.0.1 URL is used.
func openURLChecked(ctx context.Context, home string, port int, token string) (string, error) {
	pidFile := filepath.Join(panelDir(home), "pid")
	want, err := os.ReadFile(pidFile)
	if err != nil {
		return "", fmt.Errorf("no %s: the panel is not running (launchctl kickstart -k %s/%s)", pidFile, guiDomain(), LaunchdLabel)
	}
	ok := "ok pid=" + strings.TrimSpace(string(want))
	p := strconv.Itoa(port)
	v4 := "http://127.0.0.1:" + p
	got, err := healthz(ctx, v4)
	if err != nil {
		return "", fmt.Errorf("the panel is not answering on %s (%v). Restart it: launchctl kickstart -k %s/%s, and read %s",
			v4, err, guiDomain(), LaunchdLabel, LogDir(home))
	}
	if got != ok {
		return "", fmt.Errorf("%s answers /healthz with %q, not as the panel whose pid is %s; not sending it the token", v4, got, strings.TrimSpace(string(want)))
	}
	v6 := "http://[::1]:" + p
	got, err = healthz(ctx, v6)
	switch {
	case err != nil && errors.Is(err, syscall.ECONNREFUSED):
		return v4 + "/?t=" + token, nil // nothing on ::1: open the address that is ours
	case err != nil:
		return "", fmt.Errorf("cannot check %s (%v); not sending it the token", v6, err)
	case got != ok:
		return "", fmt.Errorf("%s answers /healthz with %q, not as the panel whose pid is %s. The browser resolves %s to ::1 first, "+
			"so the token is not sent", v6, got, strings.TrimSpace(string(want)), DefaultHostName)
	}
	return "http://" + DefaultHostName + ":" + p + "/?t=" + token, nil
}
