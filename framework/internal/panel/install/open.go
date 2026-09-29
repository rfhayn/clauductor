package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
)

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
	pidFile := filepath.Join(config.PanelDir(home), "pid")
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
			"so the token is not sent", v6, got, strings.TrimSpace(string(want)), config.DefaultHostName)
	}
	return "http://" + config.DefaultHostName + ":" + p + "/?t=" + token, nil
}
