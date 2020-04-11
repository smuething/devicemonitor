package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/alexbrainman/printer"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/sqweek/dialog"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

// Regex for checking if an argument needs to be quoted
var invalidChars = regexp.MustCompile(`[^-a-zA-Z0-9_=/.,:;%()?+*~\\]`)

// Regex for extracting printer name
var extractPrinter = regexp.MustCompile(`%printer%(.*)`)

// Shows an error message to users to alert them that something has gone wrong
func fatalHandler() {
	dialog.Message(`Beim Verarbeiten des Druckauftrags ist ein Fehler aufgetreten.

Für Unterstützung wenden Sie sich an Marc Diehl oder Steffen Müthing.

Bei der nächsten Fehlermeldung klicken Sie bitte auf "Nein".`).Title("Druckfehler").Error()
}

type job struct {
	args      []string
	logfile   io.Writer
	deviceArg int
	outputArg int
	device    string
	output    string
	input     string
	pdf       string
	printer   string
}

func newJob(args []string, logfile io.Writer) *job {

	deviceArg := -1
	outputArg := -1
	for i, arg := range args {
		if strings.HasPrefix(arg, "-sDEVICE=") {
			deviceArg = i
			log.Debugf("Found device specifier at position %d: %s", i, arg)
		}
		if strings.HasPrefix(arg, "-sOutputFile=") {
			outputArg = i
			log.Debugf("Found output specifier at position %d: %s", i, arg)
		}
	}

	if deviceArg < 0 {
		log.Fatal("Missing device argument")
	}

	if outputArg < 0 {
		log.Fatal("Missing output argument")
	}

	// Get printer name from GhostPCL command line
	printerName := extractPrinter.FindStringSubmatch(args[outputArg])[1]

	j := job{
		args:      args,
		logfile:   logfile,
		deviceArg: deviceArg,
		outputArg: outputArg,
		device:    args[deviceArg],
		output:    args[outputArg],
		input:     args[len(args)-1],
		pdf:       "",
		printer:   printerName,
	}

	return &j
}

func (j *job) CreatePDF() {

	j.pdf = strings.Replace(j.input, ".txt", ".pdf", 1)
	log.Infof("Creating PDF file: %s", j.pdf)

	// The wrapped executable must be late in the alphabet because Printfil picks the first executable it finds in the directory
	executable := filepath.Join(filepath.Dir(os.Args[0]), "zzz-wrapped-"+filepath.Base(os.Args[0]))
	log.Debugf("Wrapped executable: %s", executable)

	cmd := exec.Command(executable)
	cmd.Stdin = os.Stdin
	cmd.Stdout = j.logfile
	cmd.Stderr = j.logfile
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	j.args[j.deviceArg] = "-sDEVICE=pdfwrite"
	j.args[j.outputArg] = fmt.Sprintf("-sOutputFile=%s", j.pdf)

	// Escape all arguments that contain problematic characters
	for i, arg := range j.args {
		if invalidChars.MatchString(arg) {
			j.args[i] = fmt.Sprintf(`"%s"`, arg)
		}
	}

	cmdLine := strings.Join(j.args, " ")
	cmd.SysProcAttr.CmdLine = cmdLine
	log.Debugf("Calling wrapped executable with cmdline: %s", cmdLine)

	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func (j *job) ShowPDF() {

	if j.pdf == "" {
		log.Fatal("Cannot show PDF, file has not been created yet")
	}

	// Open PDF file in viewer
	log.Debug("Opening PDF file with default viewer")
	runDLL32 := filepath.Join(os.Getenv("SYSTEMROOT"), "system32", "rundll32.exe")
	cmd := exec.Command(runDLL32, "SHELL32.DLL,ShellExec_RunDLL", j.pdf)

	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
}

func (j *job) ForwardPCLStream() {

	log.Infof("Passing raw PCL data stream to printer: %s", j.printer)

	log.Debugf("Opening input file: %s", j.input)
	data, err := os.Open(j.input)
	if err != nil {
		log.Fatal(err)
	}
	defer data.Close()

	log.Debugf("Opening printer")
	p, err := printer.Open(j.printer)
	if err != nil {
	}
	defer p.Close()

	log.Debugf("Starting RAW document")
	err = p.StartDocument(filepath.Base(j.input), "RAW")
	if err != nil {
		log.Fatal(err)
	}
	defer p.EndDocument()

	log.Debugf("Starting page")
	err = p.StartPage()
	if err != nil {
		log.Fatal(err)
	}
	defer p.EndPage()

	log.Debugf("Sending file contents")
	if nBytes, err := io.Copy(p, data); err != nil {
		log.Fatal(err)
	} else {
		log.Debugf("Sent %d bytes", nBytes)
	}
}

func setupLogging() *os.File {
	logfile, err := os.OpenFile("w:\\printlog.txt", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fatalHandler()
		panic(err)
	}

	log.SetFormatter(&prefixed.TextFormatter{
		DisableColors:   true,
		ForceFormatting: true,
		FullTimestamp:   true,
	})
	log.SetOutput(logfile)
	log.RegisterExitHandler(fatalHandler)

	// Creating w:\debug.txt turns on debug logging for the current session
	_, err = os.Stat(`w:\debug.txt`)
	if !os.IsNotExist(err) {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	return logfile
}

func main() {

	logfile := setupLogging()
	defer logfile.Close()

	log.WithFields(logrus.Fields{
		"cmdline": strings.Join(os.Args, " "),
	}).Info("Startup")

	j := newJob(os.Args, logfile)

	if strings.EqualFold(j.output, "-sOutputFile=%printer%PDF") {
		j.CreatePDF()
		j.ShowPDF()
	} else {
		j.ForwardPCLStream()
	}
	log.Info("Job complete")
}
