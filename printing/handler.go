package printing

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alexbrainman/printer"
	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/monitor"
)

var (
	pitchRE *regexp.Regexp = regexp.MustCompile("\x1b\\(s([1-9][0-9]?(\\.[0-9]+))H")
)

func pclPitch(pitch int) string {
	return fmt.Sprintf("\x1b(s%dH", pitch)
}

type Job struct {
	*monitor.Job
	hasPJL         bool
	hasMultipleUEC bool
	landscape      bool
	pitchLoc       []int
	pdf            string
}

func newJob(monitorJob *monitor.Job) *Job {
	return &Job{
		Job: monitorJob,
	}
}

func (j *Job) parse() error {

	fi, err := os.Stat(j.File)
	if err != nil {
		return err
	}

	if fi.Size() > MaxJobSize {
		return fmt.Errorf("Job %s is too large, try direct forwarding to printer", j.Name)
	}

	bytes, err := ioutil.ReadFile(j.File)
	if err != nil {
		return err
	}

	data := string(bytes)

	idx := strings.Index(data, uecPJL)
	j.hasPJL = idx >= 0

	if j.hasPJL {
		j.hasMultipleUEC = idx < strings.LastIndex(data, uec)
	}

	//j.landscape = strings.Index(data, pclLandscape) >= 0

	indices := pitchRE.FindStringSubmatchIndex(data)
	if len(indices) > 0 {
		j.pitchLoc = indices[2:4]
	}

	return nil
}

func Foo(monitor *monitor.Monitor) {
	for mj := range monitor.Jobs() {
		j := newJob(mj)
		j.parse()
		j.createPDF(`w:\`)
		j.showPDF()
	}
}

func (j *Job) createPDF(path string) {

	j.pdf = j.Time.Format("Printout 2006-01-02 150405.pdf")
	j.pdf = filepath.Join(path, filepath.Base(j.pdf))
	log.Infof("Creating PDF file: %s", j.pdf)

	//scalePDF := false //config.Printing.ScaleNonPJLJobs && !j.jobContainsPJL()
	// unscaledPDF := strings.TrimSuffix(j.input, filepath.Ext(j.input)) + "-unscaled.pdf"
	// var scaleArgs []string
	// if scalePDF {
	// 	log.Infof("Assuming an oversized list, scaling from %d x %d mm to A4", config.Printing.ScaledWidth, config.Printing.ScaledHeight)
	// }

	// The wrapped executable must be late in the alphabet because Printfil picks the first executable it finds in the directory
	executable := `h:\ghostpcl-9.50-win32\zzz-wrapped-gpcl6win32.exe` //filepath.Join(filepath.Dir(os.Args[0]), "zzz-wrapped-"+filepath.Base(os.Args[0]))
	log.Debugf("Executable: %s", executable)

	args := []string{
		"-dPrinted",
		"-dBATCH",
		"-dNOPAUSE",
		"-dNOSAFER",
		"-dNumCopies=1",
		"-sDEVICE=pdfwrite",
		"-dNoCancel",
		fmt.Sprintf(`-sOutputFile=%s`, j.pdf),
		j.File,
	}

	cmd := exec.Command(executable, args...)
	cmd.Stdout = os.Stdout //j.logfile
	cmd.Stderr = os.Stdout //j.logfile

	// if scalePDF {
	// 	log.Debugf("Creating intermediate PDF file %s with nonstandard format %d x %d mm", unscaledPDF, config.Printing.ScaledWidth, config.Printing.ScaledHeight)
	// 	scaleArgs = append(j.args[:0:0], j.args...)
	// 	j.args = append(j.args, j.input)
	// 	// GhostPCL wants dimensions in dots, and uses 720 DPI by default
	// 	// our configuration uses mm for the dimensions
	// 	width := int(float64(config.Printing.ScaledWidth) * (720.0 / 25.4))
	// 	height := int(float64(config.Printing.ScaledHeight) * (720.0 / 25.4))
	// 	j.args[len(j.args)-2] = fmt.Sprintf("-g%dx%d", width, height)
	// 	j.args[j.outputArg] = fmt.Sprintf("-sOutputFile=%s", unscaledPDF)
	// } else {
	// 	j.args[j.outputArg] = fmt.Sprintf("-sOutputFile=%s", j.pdf)
	// }

	// Go messes up the command line, so we have to build it ourselves
	//cmd.SysProcAttr = &syscall.SysProcAttr{}
	//cmdLine := buildCmdLine(j.args)
	//cmd.SysProcAttr.CmdLine = cmdLine
	//log.Debugf("Calling wrapped executable with cmdline: %s", cmdLine)

	err := cmd.Run()
	if err != nil {
		log.Error(err)
	}

	// if scalePDF {

	// 	if config.Printing.KeepUnscaledPDF {
	// 		log.Debugf("Intermediate PDF %s will be kept", unscaledPDF)
	// 	} else {
	// 		log.Debugf("Intermediate PDF %s will be deleted", unscaledPDF)
	// 		defer os.Remove(unscaledPDF)
	// 	}

	// 	log.Debugf("Scaling intermediate PDF %s to DIN A4", unscaledPDF)
	// 	// Now we have to feed the generated PDF through Ghostscript to scale it to A4
	// 	ghostscript := config.Paths.GhostScript
	// 	// If no absolute path is given, we look for it in the GhostPCL directory
	// 	if !filepath.IsAbs(ghostscript) {
	// 		ghostscript = filepath.Join(filepath.Dir(os.Args[0]), ghostscript)
	// 	}

	// 	cmd = exec.Command(ghostscript)

	// 	// Ghostscript is rather chatty, so we only log its output in debug mode
	// 	if config.Debug {
	// 		cmd.Stdout = j.logfile
	// 		cmd.Stderr = j.logfile
	// 	}

	// 	scaleArgs[0] = ghostscript
	// 	scaleArgs = append(scaleArgs, unscaledPDF)
	// 	scaleArgs[j.outputArg] = fmt.Sprintf("-sOutputFile=%s", j.pdf)
	// 	scaleArgs[len(scaleArgs)-2] = "-dPDFFitPage"

	// 	cmd.SysProcAttr = &syscall.SysProcAttr{}
	// 	cmdLine = buildCmdLine(scaleArgs)
	// 	cmd.SysProcAttr.CmdLine = cmdLine
	// 	log.Debugf("Calling ghostscript with cmdline: %s", cmdLine)

	// 	err = cmd.Run()
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// }
}

func (j *Job) showPDF() {

	if j.pdf == "" {
		log.Error("Cannot show PDF, file has not been created yet")
	}

	// Open PDF file in viewer
	log.Debug("Opening PDF file with default viewer")
	runDLL32 := filepath.Join(os.Getenv("SYSTEMROOT"), "system32", "rundll32.exe")
	cmd := exec.Command(runDLL32, "SHELL32.DLL,ShellExec_RunDLL", j.pdf)

	//log.Debugf("Running: %s", buildCmdLine(cmd.Args))
	err := cmd.Start()
	if err != nil {
		log.Error(err)
	}
}

func (j *Job) forwardPCLStream() error {

	var err error
	defer func() {
		if err != nil {
			log.Error(fmt.Errorf("Error forwarding raw PCL stream to printer %s: %s", j.Printer, err))
		}
	}()

	log.Infof("Passing raw PCL data stream to printer: %s", j.Printer)

	log.Debugf("Opening input file: %s", j.File)
	data, err := os.Open(j.File)
	if err != nil {
		return err
	}
	defer data.Close()

	log.Debugf("Opening printer")
	p, err := printer.Open(j.Printer)
	if err != nil {
		return err
	}
	defer p.Close()

	log.Debugf("Starting RAW document")
	err = p.StartDocument(filepath.Base(j.File), "RAW")
	if err != nil {
		return err
	}
	defer p.EndDocument()

	log.Debugf("Starting page")
	err = p.StartPage()
	if err != nil {
		return err
	}
	defer p.EndPage()

	log.Debugf("Sending file contents")
	if nBytes, err := io.Copy(p, data); err != nil {
		return err
	} else {
		log.Debugf("Sent %d bytes", nBytes)
	}

	return nil
}

func (j *Job) printPDF() {

	log.Debugf("Sending PDF to printer %s with default settings", j.Printer)

	if j.pdf == "" {
		log.Fatal("Cannot print, PDF file has not been created yet")
	}

	viewer := `c:\Program Files\Tracker Software\PDF Viewer\PDFXCview.exe`
	cmd := exec.Command(viewer, fmt.Sprintf(`/print:default&showui=no&printer=%s`, j.Printer), j.pdf)

	// We have to manually build the command line, as Go messes up the second argument, which contains quotes
	// somewhere in the middle of the argument
	// cmd.SysProcAttr = &syscall.SysProcAttr{}
	// cmd.SysProcAttr.CmdLine = strings.Join([]string{
	// 	escapeArgument(config.Paths.PDFViewer),
	// 	fmt.Sprintf(`/print:default&showui=no&printer="%s"`, j.printer),
	// 	escapeArgument(j.pdf),
	// }, " ")
	// log.Debugf("Running: %s", cmd.SysProcAttr.CmdLine)

	if err := cmd.Run(); err != nil {
		log.Error(err)
	}
}
