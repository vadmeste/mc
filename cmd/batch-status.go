package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/minio/cli"
	json "github.com/minio/colorjson"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/mc/pkg/probe"
	"github.com/minio/pkg/v2/console"
	"github.com/olekukonko/tablewriter"
)

var batchStatusCmd = cli.Command{
	Name:            "status",
	Usage:           "summarize job events on MinIO server in real-time",
	Action:          mainBatchStatus,
	OnUsageError:    onUsageError,
	Before:          setGlobalsFromContext,
	Flags:           globalFlags,
	HideHelpCommand: true,
	CustomHelpTemplate: `NAME:
  {{.HelpName}} - {{.Usage}}

USAGE:
  {{.HelpName}} TARGET JOBID

FLAGS:
  {{range .VisibleFlags}}{{.}}
  {{end}}
EXAMPLES:
   1. Display current in-progress JOB events.
      {{.Prompt}} {{.HelpName}} myminio/ KwSysDpxcBU9FNhGkn2dCf
`,
}

// checkBatchStatusSyntax - validate all the passed arguments
func checkBatchStatusSyntax(ctx *cli.Context) {
	if len(ctx.Args()) != 2 {
		showCommandHelpAndExit(ctx, 1) // last argument is exit code
	}
}

type batchStatusMsg struct {
	m madmin.JobMetric
}

func (s batchStatusMsg) JSON() string {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)

	fatalIf(probe.NewError(enc.Encode(s)), "Unable to marshal into JSON.")
	return buf.String()
}

func (s batchStatusMsg) String() string {
	var b strings.Builder

	addLine := func(prefix string, value interface{}) {
		b.WriteString(prefix)
		b.WriteString(" ")
		b.WriteString(fmt.Sprint(value))
		b.WriteString("\n")
	}

	renderBatchJobMetrics(s.m, addLine)
	return b.String()
}

func mainBatchStatus(ctx *cli.Context) error {
	checkBatchStatusSyntax(ctx)

	aliasedURL := ctx.Args().Get(0)
	jobID := ctx.Args().Get(1)

	// Create a new MinIO Admin Client
	client, err := newAdminClient(aliasedURL)
	fatalIf(err.Trace(aliasedURL), "Unable to initialize admin client.")

	ctxt, cancel := context.WithCancel(globalContext)
	defer cancel()

	_, e := client.DescribeBatchJob(ctxt, jobID)
	nosuchJob := madmin.ToErrorResponse(e).Code == "XMinioAdminNoSuchJob"
	if nosuchJob {
		e = nil
		if !globalJSON {
			console.Infoln("Unable to find an active job, attempting to list from previously run jobs..")
			st, e := client.BatchJobStatus(ctxt, jobID)
			fatalIf(probe.NewError(e), "Unable to lookup job status")
			printMsg(batchStatusMsg{m: st.LastMetric})
			return nil
		}
	}
	fatalIf(probe.NewError(e), "Unable to lookup job status")

	ui := tea.NewProgram(initBatchJobMetricsUI(jobID))
	go func() {
		opts := madmin.MetricsOptions{
			Type:     madmin.MetricsBatchJobs,
			ByJobID:  jobID,
			Interval: time.Second,
		}
		e := client.Metrics(ctxt, opts, func(metrics madmin.RealtimeMetrics) {
			if globalJSON {
				if metrics.Aggregated.BatchJobs == nil {
					cancel()
					return
				}

				job, ok := metrics.Aggregated.BatchJobs.Jobs[jobID]
				if !ok {
					cancel()
					return
				}

				printMsg(metricsMessage{RealtimeMetrics: metrics})
				if job.Complete || job.Failed {
					cancel()
					return
				}
			} else {
				ui.Send(metrics)
			}
		})
		if e != nil && !errors.Is(e, context.Canceled) {
			fatalIf(probe.NewError(e).Trace(ctx.Args()...), "Unable to get current batch status")
		}
	}()

	if !globalJSON {
		if _, e := ui.Run(); e != nil {
			cancel()
			fatalIf(probe.NewError(e).Trace(aliasedURL), "Unable to get current batch status")
		}
	} else {
		<-ctxt.Done()
	}

	return nil
}

func initBatchJobMetricsUI(jobID string) *batchJobMetricsUI {
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &batchJobMetricsUI{
		spinner: s,
		jobID:   jobID,
	}
}

type batchJobMetricsUI struct {
	current  madmin.JobMetric
	spinner  spinner.Model
	quitting bool
	jobID    string
}

func (m *batchJobMetricsUI) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *batchJobMetricsUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		default:
			return m, nil
		}
	case madmin.RealtimeMetrics:
		metrics := msg
		if metrics.Aggregated.BatchJobs == nil {
			m.quitting = true
			return m, tea.Quit
		}

		job, ok := metrics.Aggregated.BatchJobs.Jobs[m.jobID]
		if !ok {
			m.quitting = true
			return m, tea.Quit
		}

		m.current = job
		if job.Complete || job.Failed {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func renderBatchJobMetrics(m madmin.JobMetric, fn func(prefix string, value interface{})) {
	switch m.JobType {
	case string(madmin.BatchJobReplicate):
		accElapsedTime := m.LastUpdate.Sub(m.StartTime)

		fn("JobType: ", m.JobType)
		fn("Objects: ", m.Replicate.Objects)
		fn("FailedObjects: ", m.Replicate.ObjectsFailed)
		if accElapsedTime > 0 {
			bytesTransferredPerSec := float64(m.Replicate.BytesTransferred) / accElapsedTime.Seconds()
			objectsPerSec := float64(int64(time.Second)*m.Replicate.Objects) / float64(accElapsedTime)
			fn("Throughput: ", fmt.Sprintf("%s/s", humanize.IBytes(uint64(bytesTransferredPerSec))))
			fn("IOPs: ", fmt.Sprintf("%.2f objs/s", objectsPerSec))
		}
		fn("Transferred: ", humanize.IBytes(uint64(m.Replicate.BytesTransferred)))
		fn("Elapsed: ", accElapsedTime.String())
		fn("CurrObjName: ", m.Replicate.Object)
	case string(madmin.BatchJobExpire):
		fn("JobType: ", m.JobType)
		fn("Objects: ", m.Expired.Objects)
		fn("FailedObjects: ", m.Expired.ObjectsFailed)
		fn("CurrObjName: ", m.Expired.Object)

		if !m.LastUpdate.IsZero() {
			accElapsedTime := m.LastUpdate.Sub(m.StartTime)
			fn("Elapsed: ", accElapsedTime.String())
		}
	}

}

func (m *batchJobMetricsUI) View() string {
	var s strings.Builder

	// Set table header
	table := tablewriter.NewWriter(&s)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t") // pad with tabs
	table.SetNoWhiteSpace(true)

	if !m.quitting {
		s.WriteString(m.spinner.View())
	} else {
		if m.current.Complete {
			s.WriteString(m.spinner.Style.Render((tickCell + tickCell + tickCell)))
		} else if m.current.Failed {
			s.WriteString(m.spinner.Style.Render((crossTickCell + crossTickCell + crossTickCell)))
		}
	}
	s.WriteString("\n")

	var data [][]string
	addLine := func(prefix string, value interface{}) {
		data = append(data, []string{
			prefix,
			whiteStyle.Render(fmt.Sprint(value)),
		})
	}

	renderBatchJobMetrics(m.current, addLine)

	table.AppendBulk(data)
	table.Render()

	if m.quitting {
		s.WriteString("\n")
	}
	return s.String()
}
