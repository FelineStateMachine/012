package e2e

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// serveSSH starts 012 serve on a random loopback port for served,
// authorizing a new key, and returns the system's ssh client and the
// arguments that reach the server with it, checking the host key.
func serveSSH(t *testing.T, top, served string) (string, []string) {
	t.Helper()
	sshPath, err1 := exec.LookPath("ssh")
	keygen, err2 := exec.LookPath("ssh-keygen")
	if err1 != nil || err2 != nil {
		t.Skip("needs ssh and ssh-keygen")
	}
	id := filepath.Join(top, "id_ed25519")
	if out, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-C", "e2e", "-f", id).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, err := os.ReadFile(id + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	ak := filepath.Join(top, "authorized_keys")
	if err := os.WriteFile(ak, pub, 0o600); err != nil {
		t.Fatal(err)
	}
	hostKey := filepath.Join(top, "config", "host_key")
	srv := exec.Command(binPath, "serve", "--listen", "127.0.0.1:0", "--authorized-keys", ak, "--host-key", hostKey, served)
	srv.Env = append(os.Environ(), "TYPESAFE_API_KEY=", "TYPESAFE_BASE_URL=")
	stderr, err := srv.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		srv.Process.Kill()
		srv.Wait()
	})
	// "012 serving DIR on 127.0.0.1:PORT"
	port := make(chan string, 1)
	go func() {
		re := regexp.MustCompile(` on 127\.0\.0\.1:(\d+)$`)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if m := re.FindStringSubmatch(sc.Text()); m != nil {
				port <- m[1]
			}
		}
	}()
	var p string
	select {
	case p = <-port:
	case <-time.After(waitTimeout):
		t.Fatal("012 serve didn't start")
	}
	hostPub, err := os.ReadFile(hostKey + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	knownHosts := filepath.Join(top, "known_hosts")
	os.WriteFile(knownHosts, []byte("[127.0.0.1]:"+p+" "+string(hostPub)), 0o600)
	return sshPath, []string{"-F", "/dev/null", "-i", id, "-e", "none", "-p", p,
		"-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none", "-o", "BatchMode=yes",
		"-o", "UserKnownHostsFile=" + knownHosts, "-o", "StrictHostKeyChecking=yes", "-o", "LogLevel=ERROR",
		"-t", "127.0.0.1"}
}

// 012 serve through a real ssh client in a real terminal: the first
// screen, typing, resizing, saving inside the served directory, a
// refused escape, and quitting closing the connection.
func TestServeOverSSH(t *testing.T) {
	top := t.TempDir()
	served := filepath.Join(top, "served")
	os.Mkdir(served, 0o755)
	sshPath, args := serveSSH(t, top, served)

	s := startWith(t, options{dir: top, program: sshPath}, args...)
	s.waitFor("Sheet1")
	s.keys("hello", "<enter>")
	s.waitForLine(gridRow1, "    1  hello")

	// The window follows the client's terminal.
	s.resize(120, 40)
	s.eventually("column K after resizing", func() bool { return strings.Contains(s.line(3), "  K") })

	s.keys("<ctrl+s>", "greeting", "<enter>")
	s.eventually("greeting.012 in the served directory", func() bool {
		_, err := os.Stat(filepath.Join(served, "greeting.012"))
		return err == nil
	})
	s.waitFor("greeting.012")

	s.keys("<ctrl+o>", "../id_ed25519", "<enter>")
	s.waitFor("outside the served directory")
	s.keys("<esc>")

	s.keys("<ctrl+q>")
	s.waitExit()
}
