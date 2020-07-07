package printing

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/smuething/devicemonitor/app"

	log "github.com/sirupsen/logrus"
	"github.com/smuething/devicemonitor/monitor"
)

//go:generate go-enum -type PrintLanguage -trimprefix PrintLanguage -transform kebab
type PrintLanguage int32

const (
	invalidLanguage PrintLanguage = iota
	PrintLanguagePCL
	PrintLanguagePDF
	PrintLanguagePostScript
)

//go:generate go-enum -type Orientation -trimprefix Orientation -transform kebab
type Orientation int32

const (
	invalidOrientation Orientation = iota
	OrientationPortrait
	OrientationLandscape
)

//go:generate go-enum -type JobType -trimprefix JobType -transform kebab
type JobType int32

const (
	invalidJobType JobType = iota
	JobTypeList
	JobTypeForm
)

const (
	newline            = "\r\n"
	newlineRE          = "\r?\n"
	uec                = "\x1b%-12345X"
	uecPJL             = uec + "@PJL"
	pjlLandscapePrefix = "\x1b%-12345X@PJL DEFAULT SETDISTILLERPARAMS = \"<< /AutoRotatePages /All >>\"\r"
	MaxJobSize         = 8 * (1 << 20) // 8 MiB should be plenty
)

var (
	pclSimplexDuplex    = regexp.MustCompile("\x1b&l(0|1|2)S" + newlineRE)
	pclLandscape        = regexp.MustCompile("\x1b&l1O" + newlineRE)
	pclAbsolutePosition = regexp.MustCompile("\x1b*p([1-9][0-9]*(\\.?[0-9]*))x([1-9][0-9]*(\\.?[0-9]*))Y" + newlineRE)
	pjlBlock            = regexp.MustCompile(uecPJL + ".*?ENTER LANGUAGE\\s*=\\s*PCL\\s*" + newlineRE)
)

type PrintJob struct {
	*monitor.Job
	Name        string
	Title       string
	Language    PrintLanguage
	Duplex      bool
	Tray        int
	JobType     JobType
	Orientation Orientation
	data        string
	pdf         string
	ghostPCL    string
	ghostScript string
}

func NewPrintJob(job *monitor.Job, name string, title string, language PrintLanguage, duplex bool, tray int) PrintJob {
	config := app.Config()
	config.Lock()
	defer config.Unlock()

	return PrintJob{
		Job:         job,
		Name:        name,
		Title:       title,
		Language:    language,
		Duplex:      duplex,
		Tray:        tray,
		ghostPCL:    config.Paths.GhostPCL,
		ghostScript: config.Paths.GhostScript,
	}
}

func (j *PrintJob) inspect() error {

	fi, err := os.Stat(j.File)
	if err != nil {
		return err
	}

	if fi.Size() > MaxJobSize {
		return fmt.Errorf("Could not process print job %s: file size %d exceeds max job size %d", j.File, fi.Size(), MaxJobSize)
	}

	rawData, err := ioutil.ReadFile(j.File)
	if err != nil {
		return err
	}
	j.data = string(rawData)

	if pclLandscape.FindStringIndex(j.data) != nil {
		log.Debugf("Found landscape orientation command, assuming landscape orientation")
		j.Orientation = OrientationLandscape
	} else {
		log.Debugf("No landscape orientation command found, assuming portrait orientation")
		j.Orientation = OrientationPortrait
	}

	if pclAbsolutePosition.FindStringIndex(j.data) != nil {
		if j.Orientation == OrientationLandscape {
			return fmt.Errorf("Found absolute positioning and landscape orientation in job %s, bailing out", j.File)
		} else {
			j.JobType = JobTypeList
		}
	} else {
		j.JobType = JobTypeList
	}

	return nil
}

func (j *PrintJob) NeedsScaling() bool {
	return j.JobType == JobTypeList
}

func (j *PrintJob) sanitize() error {
	if j.Orientation == OrientationLandscape {
		j.data = pclLandscape.ReplaceAllString(j.data, "")
	}
	// remove simplex and duplex commands
	j.data = pclSimplexDuplex.ReplaceAllString(j.data, "")

	// remove all PJL commands
	j.data = pjlBlock.ReplaceAllString(j.data, "")
	return nil
}

func (j *PrintJob) createPDF(path string) error {

	basename := strings.TrimSuffix(j.File, filepath.Ext(j.File))

	sanitizedName := basename + "-sanitized.txt"
	err := func() error {
		sanitized, err := os.Create(sanitizedName)
		if err != nil {
			return err
		}
		defer sanitized.Close()

		_, err = sanitized.WriteString(j.data)
		if err != nil {
			return err
		}

		return nil
	}()
	if err != nil {
		return err
	}

	j.pdf = filepath.Join(path, j.Time.Format("Printout 2006-01-02 150405.pdf"))
	log.Infof("Creating PDF file: %s", j.pdf)

	scalePDF := j.NeedsScaling()
	//unscaledPDF := basename + "-unscaled.pdf"
	//var scaleArgs []string
	if scalePDF {
		//log.Infof("Assuming an oversized list, scaling from %d x %d mm to A4", config.Printing.ScaledWidth, config.Printing.ScaledHeight)
	}

	args := []string{
		"-dPrinted",
		"-dBATCH",
		"-dNOPAUSE",
		"-dNOSAFER",
		"-dNumCopies=1",
		"-sDEVICE=pdfwrite",
		"-dNoCancel",
		fmt.Sprintf(`-sOutputFile=%s`, j.pdf),
		sanitizedName,
	}

	cmd := exec.Command(j.ghostPCL, args...)
	//cmd.Stdout = os.Stdout //j.logfile
	//cmd.Stderr = os.Stdout //j.logfile

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

	err = cmd.Run()
	if err != nil {
		return err
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

	pdfBuf, err := ioutil.ReadFile(j.pdf)
	if err != nil {
		return err
	}

	j.data = string(pdfBuf)

	return nil
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
		if _, err := out.WriteString(j.data); err != nil {
			return err
		}
	case PrintLanguagePCL:
		log.Debug("PCL job")
		if _, err := out.WriteString(j.data); err != nil {
			return err
		}
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

	/*
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
	*/
	return nil
}

func (j *PrintJob) Process() {
	j.inspect()
	j.sanitize()
	j.createPDF("w:\\")
	j.sendToPrinter()
}
