// Copyright (c) 2015-2022 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/olekukonko/tablewriter"
)

type topDiskStats struct {
	mu         sync.Mutex
	apiCalls   map[string]uint64
	apiLatency map[string]uint64
}

func (s *topDiskStats) setAPICall(storage string, n uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.apiCalls[storage] = n
}

func (s *topDiskStats) setAPILatency(storage string, n uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.apiLatency[storage] = n
}

func (s *topDiskStats) loadAPICall(storage string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.apiCalls[storage]
}

func (s *topDiskStats) loadAPILatency(storage string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.apiLatency[storage]
}

func (s *topDiskStats) availableAPIs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := 0
	calls := make([]string, len(s.apiCalls))
	for k := range s.apiCalls {
		calls[i] = k
		i++
	}
	return calls
}

type diskTraceUI struct {
	spinner   spinner.Model
	quitting  bool
	startTime time.Time
	result    topDiskResult

	statMu   sync.Mutex
	statsMap map[string]*topDiskStats
}

type topDiskResult struct {
	final       bool
	diskName    string
	diskAPIName string
	diskAPICall uint64
	diskAPIAvg  uint64
}

func initTopDiskUI() *diskTraceUI {
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &diskTraceUI{
		spinner:  s,
		statsMap: make(map[string]*topDiskStats),
	}
}

func (m *diskTraceUI) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *diskTraceUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		default:
			return m, nil
		}
	case topDiskResult:
		if msg.final {
			m.quitting = true
			return m, tea.Quit
		}

		m.statMu.Lock()
		s := m.statsMap[msg.diskName]
		if s == nil {
			s = &topDiskStats{
				apiCalls:   make(map[string]uint64),
				apiLatency: make(map[string]uint64),
			}
		}
		m.statsMap[msg.diskName] = s
		m.statMu.Unlock()

		s.setAPICall(msg.diskAPIName, msg.diskAPICall)
		s.setAPILatency(msg.diskAPIName, msg.diskAPIAvg)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m *diskTraceUI) View() string {
	var s strings.Builder
	s.WriteString("\n")

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

	table.SetHeader([]string{"DISK", "CALL", "COUNT", "LATENCY"})
	data := make([][]string, 0, len(m.statsMap))

	m.statMu.Lock()
	for disk, stats := range m.statsMap {
		for _, call := range stats.availableAPIs() {
			data = append(data, []string{
				disk,
				call,
				whiteStyle.Render(fmt.Sprintf("%d", stats.loadAPICall(call))),
				whiteStyle.Render(time.Duration(stats.loadAPILatency(call)).String()),
			})
		}
	}
	m.statMu.Unlock()

	sort.Slice(data, func(i, j int) bool {
		switch {
		case data[i][1] == data[j][1]:
			return data[i][0] < data[j][0]
		default:
			return data[i][1] < data[j][1]
		}
	})

	table.AppendBulk(data)
	table.Render()

	if !m.quitting {
		s.WriteString(fmt.Sprintf("\nTop disk: %s", m.spinner.View()))
	}
	return s.String()
}
