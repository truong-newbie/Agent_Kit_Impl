package cli

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"dtkit.local/dtkit/internal/paths"
)

// Deps holds injected runtime boundaries for CLI commands.
type Deps struct {
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	IsInteractive func() bool
	Env           paths.Environment
	Now           func() time.Time
	LookPath      func(string) (string, error)
	Verbose       bool
}

func DefaultDeps(stdin io.Reader, stdout, stderr io.Writer) Deps {
	return Deps{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		IsInteractive: func() bool {
			f, ok := stdin.(*os.File)
			if !ok {
				return false
			}
			info, err := f.Stat()
			if err != nil {
				return false
			}
			return info.Mode()&os.ModeCharDevice != 0
		},
		Env:      paths.DefaultEnvironment(),
		Now:      time.Now,
		LookPath: exec.LookPath,
	}
}

func (d Deps) withDefaults() Deps {
	if d.Stdout == nil {
		d.Stdout = io.Discard
	}
	if d.Stderr == nil {
		d.Stderr = io.Discard
	}
	if d.IsInteractive == nil {
		d.IsInteractive = func() bool { return false }
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.LookPath == nil {
		d.LookPath = exec.LookPath
	}
	return d
}

func confirmPrompt(ctx context.Context, deps Deps, prompt string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if _, err := io.WriteString(deps.Stderr, prompt+" [y/N]: "); err != nil {
		return false, err
	}
	if deps.Stdin == nil {
		return false, nil
	}
	reader := bufio.NewReader(deps.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
