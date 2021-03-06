package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
)

var path string
var args []string
var buildTags string
var includeVendor bool
var wd string

func main() {
	args = parseArgs()

	var err error
	path, err = os.Getwd()
	if err != nil {
		log.Fatalf("could not get current working directory: %v\n", err)
	}

	watch(runCmd())
}

func parseArgs() []string {
	args := []string{}
	for _, s := range os.Args {
		if strings.HasPrefix(s, "--build-tags=") {
			buildTags = strings.Split(s, "=")[1]
			continue
		} else if strings.HasPrefix(s, "--include-vendor") {
			includeVendor = true
			continue
		} else if strings.HasPrefix(s, "--watch-dir=") {
			wd = strings.Split(s, "=")[1]
			continue
		}

		args = append(args, s)
	}

	return args
}

func killCmd(cmd *exec.Cmd) error {
	if err := cmd.Process.Kill(); err != nil {
		return err
	}

	_, err := cmd.Process.Wait()
	return err
}

func runCmd() *exec.Cmd {
	_, dirName := filepath.Split(path)
	buildArgs := []string{"build"}
	if buildTags != "" {
		buildArgs = append(buildArgs, "-tags", buildTags)
	}

	sub := exec.Command("go", buildArgs...)
	sub.Dir = path
	_, err := sub.Output()
	if err != nil {
		switch err.(type) {
		case *exec.ExitError:
			log.Fatal(string(err.(*exec.ExitError).Stderr))
		default:
			log.Fatal(err)
		}
	}

	cmd := exec.Command("./" + dirName)
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Args = append(cmd.Args, args[1:]...)
	cmd.Env = os.Environ()

	err = cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	return cmd
}

func watch(cmd *exec.Cmd) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	go func() {
		for event := range watcher.Events {
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Println(color.MagentaString("modified file: %v", event.Name))
				if cmdErr := killCmd(cmd); cmdErr != nil {
					log.Fatal(cmdErr)
				}
				cmd = runCmd()
			}
		}
	}()

	errs := []error{}
	if wd == "" {
		wd = path
	}

	files := getFiles(wd)
	for _, p := range files {
		errs = append(errs, watcher.Add(p))
	}

	for _, err = range errs {
		if err != nil {
			log.Fatal(err)
		}
	}

	<-make(chan struct{})
}

func getFiles(path string) []string {
	results := []string{}
	folder, err := os.Open(path)
	if err != nil {
		log.Fatalf("could not open watch dir: %v", err)
	}

	defer folder.Close()

	files, _ := folder.Readdir(-1)
	for _, file := range files {
		fileName := file.Name()
		newPath := path + "/" + fileName

		isValidDir := file.IsDir() && !strings.HasPrefix(fileName, ".")

		if !includeVendor {
			isValidDir = isValidDir && fileName != "vendor"
		}

		isValidFile := !file.IsDir() &&
			strings.HasSuffix(fileName, ".go") &&
			!strings.HasSuffix(fileName, "_test.go")

		if isValidDir {
			results = append(results, getFiles(newPath)...)
		} else if isValidFile {
			results = append(results, newPath)
		}
	}

	return results
}
