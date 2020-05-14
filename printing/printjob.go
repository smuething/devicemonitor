package printing

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/alexbrainman/printer"
	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/monitor"
)

//go:generate go-enum -type PrintLanguage
type PrintLanguage int32

const (
	invalidLanguage PrintLanguage = iota
	PrintLanguagePCL
	PrintLanguagePDF
	PrintLanguagePostScript
)

//go:generate go-enum -type Orientation
type Orientation int32

const (
	invalidOrientation Orientation = iota
	OrientationPortrait
	OrientationLandscape
)

const (
	newline            = "\r\n"
	uec                = "\x1b%-12345X"
	uecPJL             = uec + "@PJL"
	pjlLandscapePrefix = "\x1b%-12345X@PJL DEFAULT SETDISTILLERPARAMS = \"<< /AutoRotatePages /All >>\"\r"
	pclLandscape       = "\x1b&l1O"
	maxJobSize         = 8 * (1 << 20) // 8 MiB should be plenty
)

type PrintJob struct {
	*monitor.Job
	Name        string
	Title       string
	Language    PrintLanguage
	Duplex      bool
	Tray        int
	Orientation Orientation
	data        io.Reader
	pdf         string
}

func NewPrintJob(name string, title string, language PrintLanguage, duplex bool, tray int, orientation Orientation, data io.Reader) PrintJob {
	return PrintJob{
		Name:        name,
		Title:       title,
		Language:    language,
		Duplex:      duplex,
		Tray:        tray,
		Orientation: orientation,
		data:        data,
	}
}

func (j *PrintJob) NeedsScaling() bool {
	return false
}

func (j *PrintJob) createPDF(path string) {

	j.pdf = j.Time.Format("Printout 2006-01-02 150405.pdf")
	j.pdf = filepath.Join(path, filepath.Base(j.pdf))
	log.Infof("Creating PDF file: %s", j.pdf)

	scalePDF := j.NeedsScaling() // false //config.Printing.ScaleNonPJLJobs && !j.jobContainsPJL()
	//unscaledPDF := strings.TrimSuffix(j.File, filepath.Ext(j.File)) + "-unscaled.pdf"
	//var scaleArgs []string
	if scalePDF {
		//log.Infof("Assuming an oversized list, scaling from %d x %d mm to A4", config.Printing.ScaledWidth, config.Printing.ScaledHeight)
	}

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

	//Go messes up the command line, so we have to build it ourselves
	// cmd.SysProcAttr = &syscall.SysProcAttr{}
	// cmdLine := buildCmdLine(j.args)
	// cmd.SysProcAttr.CmdLine = cmdLine
	// log.Debugf("Calling wrapped executable with cmdline: %s", cmdLine)

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

func (j *PrintJob) spool(out *bufio.Writer) error {
	write := func(format string, data ...interface{}) {
		fmt.Fprintf(out, format, data...)
		out.WriteString(newline)
	}
	writeRaw := func(format string, data ...interface{}) {
		fmt.Fprintf(out, format, data...)
	}

	write(uecPJL)
	write(`@PJL JOB NAME = "%s" DISPLAY = "%s"`, j.Name, j.Title)

	if j.Tray > 0 {
		log.Debugf("Printing from tray %s", j.Tray)
		write(`@PJL SET MEDIASOURCE = TRAY%d`, j.Tray)
	}

	if j.Duplex {
		write(`@PJL SET DUPLEX = ON`)
	} else {
		write(`@PJL SET DUPLEX = OFF`)
	}

	if j.Orientation != invalidOrientation {
		write(`@PJL SET ORIENTATION = %s`, j.Orientation)
	}

	write(`@PJL ENTER LANGUAGE = %s`, j.Language)

	switch j.Language {
	case PrintLanguagePDF:
		log.Debug("PDF job: forwarding PDF payload unchanged")
		io.Copy(out, j.data)
	case PrintLanguagePCL:
		log.Debug("PCL job")
		io.Copy(out, j.data)
	default:
		return fmt.Errorf("Unknown job type: %d (%s)", j.Language, j.Language)
	}

	write(uecPJL)
	write(`@PJL RESET`)
	write(`@PJL EOJ NAME = "%s"`, j.Name)
	writeRaw(uec)
	return nil
}

func (j *PrintJob) sendToPrinter() error {

	var err error
	defer func() {
		if err != nil {
			log.Error(fmt.Errorf("Error forwarding data stream to printer %s: %s", j.Printer, err))
		}
	}()

	log.Infof("Passing data stream to printer: %s", j.Printer)

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

	log.Debugf("Sending data")
	bufferedWriter := bufio.NewWriter(p)
	if err := j.spool(bufferedWriter); err != nil {
		return err
	}
	if err := bufferedWriter.Flush(); err != nil {
		return err
	}

	return nil
}
