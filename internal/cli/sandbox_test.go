package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/metorial/cli/internal/app"
	"github.com/metorial/cli/internal/sandboxaccess/sandboxtest"
	"github.com/metorial/cli/internal/terminal"
)

type sandboxRun struct {
	code   int
	stdout string
	stderr string
}

func runSandboxAdmin(t *testing.T, server *sandboxtest.Server, stdin string, args ...string) sandboxRun {
	t.Helper()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	application := app.NewWithIO(strings.NewReader(stdin), stdout, stderr, terminal.Features{}, terminal.Features{})
	full := append([]string{"--api-key", "metorial_sk_test", "--api-host", server.URL}, args...)
	code := RunAdminArgs(application, full)
	return sandboxRun{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestAdminSandboxMachinesHelpListsShellAndFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := sandboxtest.New(t)
	help := runSandboxAdmin(t, server, "", "sandbox", "machines", "--help")
	for _, expected := range []string{"shell", "Open a shell on a sandbox machine", "files", "Browse, download, and upload files"} {
		if !strings.Contains(help.stdout, expected) {
			t.Fatalf("machines help missing %q:\n%s", expected, help.stdout)
		}
	}

	files := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "--help")
	for _, expected := range []string{"ls", "stat", "cat", "download", "upload", "rm", "mkdir", "mv"} {
		if !strings.Contains(files.stdout, "  "+expected+" ") {
			t.Fatalf("files help missing %q:\n%s", expected, files.stdout)
		}
	}
}

func TestAdminSandboxFilesTransfersDirectoriesBothWays(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := sandboxtest.New(t)
	local := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(filepath.Join(local, "src", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "README.md"), []byte("# app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "src", "main.sh"), []byte("echo hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	server.PutDir("/work")

	upload := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "upload", "sbm_1", local, "/work", "-r")
	if upload.code != 0 {
		t.Fatalf("upload exit = %d, stderr = %s", upload.code, upload.stderr)
	}
	if !strings.Contains(upload.stderr, "uploaded 2 files") {
		t.Fatalf("upload summary = %q", upload.stderr)
	}
	if node, ok := server.Node("/work/app/src/main.sh"); !ok || string(node.Data) != "echo hi\n" || node.Mode != 0o755 {
		t.Fatalf("uploaded main.sh = %+v, %v", node, ok)
	}
	if node, ok := server.Node("/work/app/src/empty"); !ok || !node.Dir {
		t.Fatalf("empty directory was not created: %+v, %v", node, ok)
	}

	list := runSandboxAdmin(t, server, "", "--format", "json", "sandbox", "machines", "files", "ls", "sbm_1", "/work/app")
	if list.code != 0 {
		t.Fatalf("ls exit = %d, stderr = %s", list.code, list.stderr)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(list.stdout), &entries); err != nil {
		t.Fatalf("ls output is not json: %v\n%s", err, list.stdout)
	}
	if len(entries) != 2 || entries[0]["name"] != "README.md" || entries[1]["name"] != "src" {
		t.Fatalf("ls entries = %v", entries)
	}

	table := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "ls", "sbm_1", "/work/app")
	if !strings.Contains(table.stdout, "-rw-r--r--") || !strings.Contains(table.stdout, " src/") {
		t.Fatalf("ls table = %q", table.stdout)
	}

	cat := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "cat", "sbm_1", "/work/app/README.md")
	if cat.code != 0 || cat.stdout != "# app\n" {
		t.Fatalf("cat = %+v", cat)
	}

	target := t.TempDir()
	download := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "download", "sbm_1", "/work/app", target, "-r")
	if download.code != 0 {
		t.Fatalf("download exit = %d, stderr = %s", download.code, download.stderr)
	}
	script, err := os.ReadFile(filepath.Join(target, "app", "src", "main.sh"))
	if err != nil || string(script) != "echo hi\n" {
		t.Fatalf("downloaded main.sh = %q, %v", script, err)
	}
	if info, err := os.Stat(filepath.Join(target, "app", "src", "main.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("downloaded main.sh mode = %v, %v", info, err)
	}
	if info, err := os.Stat(filepath.Join(target, "app", "src", "empty")); err != nil || !info.IsDir() {
		t.Fatalf("downloaded empty dir = %v, %v", info, err)
	}

	directory := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "download", "sbm_1", "/work/app", target)
	if directory.code == 0 || !strings.Contains(directory.stderr, "pass -r") {
		t.Fatalf("download without -r = %+v", directory)
	}

	stdinUpload := runSandboxAdmin(t, server, "from stdin", "sandbox", "machines", "files", "upload", "sbm_1", "-", "/work/stdin.txt")
	if node, ok := server.Node("/work/stdin.txt"); stdinUpload.code != 0 || !ok || string(node.Data) != "from stdin" {
		t.Fatalf("stdin upload = %+v, node = %+v", stdinUpload, node)
	}

	if run := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "mv", "sbm_1", "/work/stdin.txt", "/work/moved.txt"); run.code != 0 {
		t.Fatalf("mv = %+v", run)
	}
	if run := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "mkdir", "sbm_1", "/work/new/deep"); run.code != 0 {
		t.Fatalf("mkdir = %+v", run)
	}
	if run := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "rm", "sbm_1", "/work/moved.txt", "/work/app", "-r"); run.code != 0 {
		t.Fatalf("rm = %+v", run)
	}
	if _, ok := server.Node("/work/moved.txt"); ok {
		t.Fatal("moved.txt still exists")
	}
	if _, ok := server.Node("/work/app/README.md"); ok {
		t.Fatal("app was not deleted recursively")
	}
	if node, ok := server.Node("/work/new/deep"); !ok || !node.Dir {
		t.Fatal("mkdir did not create the directory")
	}

	relative := runSandboxAdmin(t, server, "", "sandbox", "machines", "files", "cat", "sbm_1", "work/x")
	if relative.code == 0 || !strings.Contains(relative.stderr, "must be absolute") {
		t.Fatalf("relative path = %+v", relative)
	}
}

func TestAdminSandboxShellExecRelaysOutputAndExitCode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := sandboxtest.New(t)
	server.Shell = func(conn *websocket.Conn, first map[string]any) {
		command, _ := json.Marshal(first["command"])
		env, _ := json.Marshal(first["env"])
		if first["tty"] != false || string(command) != `["ls","-la"]` || string(env) != `{"FOO":"bar"}` {
			t.Errorf("open frame = %v", first)
		}
		sandboxtest.WriteJSON(conn, map[string]any{"type": "session_init", "sessionId": "shs_1", "tty": false, "offset": 0})
		_ = conn.Write(context.Background(), websocket.MessageBinary, append([]byte{0x01}, "listing\n"...))
		_ = conn.Write(context.Background(), websocket.MessageBinary, append([]byte{0x02}, "warning\n"...))
		sandboxtest.WriteJSON(conn, map[string]any{"type": "exit", "code": 7, "signal": nil})
		_ = conn.Close(websocket.StatusNormalClosure, "exited")
	}

	run := runSandboxAdmin(t, server, "", "sandbox", "machines", "shell", "sbm_1", "-e", "FOO=bar", "--", "ls", "-la")
	if run.code != 7 {
		t.Fatalf("exit = %d, stderr = %s", run.code, run.stderr)
	}
	if run.stdout != "listing\n" || run.stderr != "warning\n" {
		t.Fatalf("stdout = %q, stderr = %q", run.stdout, run.stderr)
	}
	if len(server.AccessRequests) != 1 || server.AccessRequests[0] != "sbm_1:shell" {
		t.Fatalf("access requests = %v", server.AccessRequests)
	}
}
