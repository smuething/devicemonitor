package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var invalidChars = regexp.MustCompile(`[^-a-zA-Z0-9_=/.,:;%()?+*]~`)

func main() {
	logfile, err := os.OpenFile("w:\\arglog.txt", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}
	defer logfile.Close()

	fmt.Fprintf(logfile, "\r\noriginal: %s\r\n", strings.Join(os.Args, " "))
	executable := filepath.Join(filepath.Dir(os.Args[0]), "zzz-wrapped-"+filepath.Base(os.Args[0]))
	cmd := exec.Command(executable)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	args := os.Args
	deviceArg := -1
	outputArg := -1
	pdf := ""
	printingToPDF := false
	creatingPDF := false
	for i, arg := range args {
		if strings.HasPrefix(arg, "-sDEVICE=") {
			deviceArg = i
			if strings.EqualFold(arg, "-sDEVICE=pdfwrite") {
				creatingPDF = true
			}
		}
		if strings.HasPrefix(arg, "-sOutputFile=") {
			outputArg = i
			if strings.EqualFold(arg, "-sOutputFile=%printer%PDF") {
				printingToPDF = true
			}
		}
	}

	if !creatingPDF {
		if printingToPDF {
			args[deviceArg] = "-sDEVICE=pdfwrite"
			pdf = strings.Replace(args[len(args)-1], ".txt", ".pdf", 1)
			args[outputArg] = fmt.Sprintf("-sOutputFile=%s", pdf)
		} else {
			args[deviceArg] = "-sDEVICE=pxlmono"
		}
	}

	// Escape all arguments that contain whitespace
	for i, arg := range args {
		if invalidChars.MatchString(arg) {
			args[i] = fmt.Sprintf(`"%s"`, arg)
		}
	}

	cmdLine := strings.Join(args, " ")
	// cmd.SysProcAttr.CmdLine = cmdLine
	fmt.Fprintf(logfile, "calling with: %s\r\n", cmdLine)

	err = cmd.Run()
	if err != nil {
		panic(err)
	}

	if printingToPDF {
		runDLL32 := filepath.Join(os.Getenv("SYSTEMROOT"), "system32", "rundll32.exe")
		cmd = exec.Command(runDLL32, pdf)
		cmd.Start()
	}
}
