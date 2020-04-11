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

// PDFViewer path to PDF viewer executable
const PDFViewer = `c:\Program Files\Tracker Software\PDF Viewer\PDFXCview.exe`

// Shows an error message to users to alert them that something has gone wrong
func fatalHandler() {
	dialog.Message(`Beim Verarbeiten des Druckauftrags ist ein Fehler aufgetreten.

Für Unterstützung wenden Sie sich an Marc Diehl oder Steffen Müthing.

Bei der nächsten Fehlermeldung klicken Sie bitte auf "Nein".`).Title("Druckfehler").Error()
}

func escapeArgument(arg string) string {
	if invalidChars.MatchString(arg) {
		arg = fmt.Sprintf(`"%s"`, arg)
	}
	return arg
}

func buildCmdLine(args []string) string {
	// Escape all arguments that contain problematic characters
	for i, arg := range args {
		args[i] = escapeArgument(arg)
	}

	return strings.Join(args, " ")
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

	j.args[j.deviceArg] = "-sDEVICE=pdfwrite"
	j.args[j.outputArg] = fmt.Sprintf("-sOutputFile=%s", j.pdf)

	// Go messes up the command line, so we have to build it ourselves
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmdLine := buildCmdLine(j.args)
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

	log.Debugf("Running: %s", buildCmdLine(cmd.Args))
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

func (j *job) PrintPDF() {

	log.Debugf("Sending PDF to printer %s with default settings", j.printer)

	if j.pdf == "" {
		log.Fatal("Cannot print, PDF file has not been created yet")
	}

	cmd := exec.Command(PDFViewer, fmt.Sprintf(`/print:default&showui=no&printer="%s"`, j.printer), j.pdf)

	// We have to manually build the command line, as Go messes up the second argument, which contains quotes
	// somewhere in the middle of the argument
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.CmdLine = strings.Join([]string{
		escapeArgument(PDFViewer),
		fmt.Sprintf(`/print:default&showui=no&printer="%s"`, j.printer),
		escapeArgument(j.pdf),
	}, " ")
	log.Debugf("Running: %s", cmd.SysProcAttr.CmdLine)

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func (j *job) createPDFJSFile() string {

	jspath := strings.Replace(j.input, ".txt", ".js", 1)
	log.Debugf("Creating JavaScript file %s for PDF Viewer", jspath)
	jsfile, err := os.Create(jspath)
	if err != nil {
		log.Fatal(err)
	}
	defer jsfile.Close()

	fmt.Fprintf(jsfile, "this.print({bUI:true,bSilent:true,bShrinkToFit:false});\r\n")

	return jspath
}

func (j *job) PrintPDFSelectPrinter() {

	log.Debugf("Opening PDF viewer print dialog")

	if j.pdf == "" {
		log.Fatal("Cannot print, PDF file has not been created yet")
	}

	js := j.createPDFJSFile()
	defer os.Remove(js)

	cmd := exec.Command(PDFViewer, "/runjs:showui=no", js, j.pdf)
	log.Debugf("Running: %s", buildCmdLine(cmd.Args))
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
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

	// Parse job information from GhostPCL command line
	j := newJob(os.Args, logfile)

	switch j.printer {
	case "PDF":
		log.Infof("Mode: Creating PDF and showing on screen")
		j.CreatePDF()
		j.ShowPDF()
	case "Drucker wählen":
		// As we don't know what kind of printer (local or TS redirected) the user will
		// choose, we always go through an intermediate PDF. This has the added advantage
		// of honoring any print settings made by the user - due to the way PCL-based printing
		// works in Printfil, the settings picked in Printfil's printer selection dialog are
		// directly discarded and the print job uses the default print settings.
		log.Infof("Mode: Creating PDF and showing PDF viewer print dialog")
		j.CreatePDF()
		j.PrintPDFSelectPrinter()
	default:
		if strings.Contains(j.printer, "umgeleitet") {
			// Redirected printers go through some crazy hoops transmitting the print
			// data to the TS client. The PCL stream does not survive this process, so
			// we need to render to PDF and then print the PDF
			log.Infof("Mode: directly printing to TS redirected printer: %s", j.printer)
			j.CreatePDF()
			j.PrintPDF()
		} else {
			// We assume that all directly connected printers support PCL, so there's no point
			// in processing the data stream
			log.Infof("Mode: forwarding raw PCL data to printer: %s", j.printer)
			j.ForwardPCLStream()
		}
	}
	log.Info("Job complete")
}
