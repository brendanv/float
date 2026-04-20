package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// SnapshotsTab lists git snapshots and allows restoring to a prior state.
type SnapshotsTab struct {
	width, height int
	client        floatv1connect.LedgerServiceClient
	styles        Styles

	panelBase
	snapshots []*floatv1.Snapshot
	table     table.Model

	// non-empty while restore confirmation is shown
	confirmRestoreHash string
	restoreErrMsg      string
	restoring          bool
}

func NewSnapshotsTab(client floatv1connect.LedgerServiceClient, st Styles) SnapshotsTab {
	return SnapshotsTab{
		client:    client,
		styles:    st,
		panelBase: newPanelBase(),
		table:     newSnapshotsTable(st),
	}
}

func (m SnapshotsTab) setStyles(st Styles) SnapshotsTab {
	m.styles = st
	m.table.SetStyles(styledTableStyles(st))
	return m
}

func (m SnapshotsTab) SetSize(w, h int) SnapshotsTab {
	m.width = w
	m.height = h
	m.panelBase.width = w
	m.panelBase.height = h
	m.table.SetWidth(w)
	m.table.SetHeight(h)
	m.resizeColumns()
	return m
}

func (m *SnapshotsTab) resizeColumns() {
	hashW := 14
	dateW := 22
	msgW := m.width - hashW - dateW - 1
	if msgW < 10 {
		msgW = 10
	}
	m.table.SetColumns([]table.Column{
		{Title: "Hash", Width: hashW},
		{Title: "Date", Width: dateW},
		{Title: "Message", Width: msgW},
	})
}

func (m SnapshotsTab) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick(), FetchSnapshots(m.client))
}

func (m SnapshotsTab) KeyMap() help.KeyMap {
	if m.confirmRestoreHash != "" {
		return RestoreConfirmKeyMap{}
	}
	return SnapshotsListKeyMap{}
}

func (m SnapshotsTab) Update(msg tea.Msg) (SnapshotsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case SnapshotsMsg:
		if msg.Err != nil {
			m.SetError(msg.Err.Error())
		} else {
			m.setSnapshots(msg.Snapshots)
		}
		return m, nil

	case RestoreSnapshotMsg:
		m.restoring = false
		if msg.Err != nil {
			m.restoreErrMsg = msg.Err.Error()
		} else {
			m.restoreErrMsg = ""
			// Reload the snapshot list after a successful restore.
			m.panelBase = newPanelBase()
			return m, tea.Batch(m.spinner.Tick(), FetchSnapshots(m.client))
		}
		return m, nil

	case tea.KeyMsg:
		// Confirmation dialog intercepts all keys.
		if m.confirmRestoreHash != "" {
			switch msg.String() {
			case "y":
				hash := m.confirmRestoreHash
				m.confirmRestoreHash = ""
				m.restoring = true
				return m, RestoreSnapshotCmd(m.client, hash)
			case "esc", "n":
				m.confirmRestoreHash = ""
				m.restoreErrMsg = ""
			}
			return m, nil
		}

		switch msg.String() {
		case "r":
			m.panelBase = newPanelBase()
			return m, tea.Batch(m.spinner.Tick(), FetchSnapshots(m.client))
		case "enter":
			if snap := m.selectedSnapshot(); snap != nil && !m.restoring {
				m.confirmRestoreHash = snap.Hash
				m.restoreErrMsg = ""
			}
			return m, nil
		default:
			if m.state == stateLoaded {
				var cmd tea.Cmd
				m.table, cmd = m.table.Update(msg)
				return m, cmd
			}
		}
	default:
		cmd := m.handleSpinnerTick(msg)
		return m, cmd
	}
	return m, nil
}

func (m SnapshotsTab) View() string {
	if m.confirmRestoreHash != "" {
		snap := m.snapshotByHash(m.confirmRestoreHash)
		content := fmt.Sprintf("Restore data to snapshot %s?", m.confirmRestoreHash[:12])
		if snap != nil && snap.Message != "" {
			content += "\n\n" + snap.Message
		}
		content += "\n\n[y] confirm  [esc] cancel"
		return RenderModal(m.width, m.height, "Confirm Restore", content, m.styles)
	}

	switch m.state {
	case stateLoading:
		return m.renderLoading()
	case stateError:
		return m.renderError(true)
	case stateLoaded:
		if len(m.snapshots) == 0 {
			return lipgloss.NewStyle().
				Width(m.width).Height(m.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No snapshots found.")
		}
		body := m.table.View()
		if m.restoreErrMsg != "" {
			errLine := m.styles.Error.Render("! " + m.restoreErrMsg)
			body = lipgloss.JoinVertical(lipgloss.Left, body, errLine)
		}
		return body
	}
	return ""
}

func (m *SnapshotsTab) setSnapshots(snaps []*floatv1.Snapshot) {
	m.snapshots = snaps
	m.state = stateLoaded
	rows := make([]table.Row, len(snaps))
	for i, s := range snaps {
		hash := s.Hash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		date := formatSnapshotTime(s.Timestamp)
		rows[i] = table.Row{hash, date, s.Message}
	}
	m.table.SetRows(rows)
}

func (m SnapshotsTab) selectedSnapshot() *floatv1.Snapshot {
	if len(m.snapshots) == 0 {
		return nil
	}
	c := m.table.Cursor()
	if c < 0 || c >= len(m.snapshots) {
		return nil
	}
	return m.snapshots[c]
}

func (m SnapshotsTab) snapshotByHash(hash string) *floatv1.Snapshot {
	for _, s := range m.snapshots {
		if s.Hash == hash {
			return s
		}
	}
	return nil
}

func formatSnapshotTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func newSnapshotsTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Hash", Width: 14},
			{Title: "Date", Width: 22},
			{Title: "Message", Width: 40},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}
