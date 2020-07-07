package printing

import (
	"github.com/alexbrainman/printer"
	log "github.com/sirupsen/logrus"
)

type PrintServer struct {
}

type PrintResult struct {
	Message string
	Error   error
}

func (s *PrintServer) Print(job *ServerJob, result *PrintResult) error {

	log.Debugf("logging")

	printer.Open(job.Job.Printer)

	result = &PrintResult{Message: "Success"}

	return nil
}
