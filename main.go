// go-latest tries to upgrade programs go install-d to GOBIN.
//
// For reference, go itself:
// ./src/cmd/go/internal/version/version.go
// ./src/cmd/go/internal/work/build.go
package main

import (
	"context"
	"debug/buildinfo"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
)

func gobin() string {
	gobin := os.Getenv("GOBIN")
	if gobin != "" {
		return gobin
	}
	home := os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, "go", "bin")
	}
	return ""
}

func listPrograms(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var programs []string
	for _, fi := range files {
		if isExecutable(fi) {
			programs = append(programs, filepath.Join(dir, fi.Name()))
		}
	}
	return programs, nil
}

func isExecutable(fi fs.FileInfo) bool {
	return fi.Mode().Perm()&0111 != 0
}

func latest(ctx context.Context) error {
	dir := gobin()
	if dir == "" {
		return errors.New("GOBIN not found")
	}
	progs, err := listPrograms(dir)
	if err != nil {
		return err
	}

	for _, f := range progs {
		// TODO: Is it faster to combine packages from the same module into a single exec?
		info, err := buildinfo.ReadFile(f)
		if err != nil {
			return err
		}
		// TODO: Print "pkg before -> after"
		fmt.Printf("%s %s\n", info.Path, info.Main.Version)

		if info.Main.Version == "(devel)" {
			continue
		}
		// TODO: Also skip if installed at specific SHA.

		cmd := exec.CommandContext(ctx, "go", "install", info.Path+"@latest")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("go install failed (%s):\n%s", err, out)
		}
		// TODO: If no longer present in module, ask if remove?
	}

	return nil
}

func runMain() error {
	showVersion := flag.Bool("v", false, "print version")
	flag.Parse()

	if *showVersion {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return errors.New("could not read buildinfo")
		}
		fmt.Println(bi.Main.Version)
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	err := latest(ctx)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	err := runMain()
	if err != nil {
		fmt.Printf("%s", err.Error())
		os.Exit(1)
	}
}
