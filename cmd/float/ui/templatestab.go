package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type templatesMode int

const (
	templatesModeList templatesMode = iota
	templatesModeDetail
)

// TemplatesTab lets TUI users browse saved transaction templates and remove stale ones.
type TemplatesTab struct {
	width, height int
	client        floatv1connect.LedgerServiceClient
	styles        Styles
	mode          templatesMode

	panelBase
	templates []*floatv1.TransactionTemplate
	table     table.Model

	confirmDeleteID string
	deleteErrMsg    string
}

func NewTemplatesTab(client floatv1connect.LedgerServiceClient, st Styles) TemplatesTab {
	return TemplatesTab{
		client:    client,
		styles:    st,
		panelBase: newPanelBase(),
		table:     newTemplatesTable(st),
	}
}

func newTemplatesTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Name", Width: 24},
			{Title: "Payee", Width: 24},
			{Title: "Postings", Width: 8},
			{Title: "Tags", Width: 16},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func (m TemplatesTab) setStyles(st Styles) TemplatesTab {
	m.styles = st
	m.table.SetStyles(styledTableStyles(st))
	return m
}

func (m TemplatesTab) SetSize(w, h int) TemplatesTab {
	m.width = w
	m.height = h
	m.panelBase.width = w
	m.panelBase.height = h
	m.table.SetWidth(w)
	m.table.SetHeight(h)
	m.resizeColumns()
	return m
}

func (m *TemplatesTab) resizeColumns() {
	postingsW := 8
	tagsW := 16
	remaining := m.width - postingsW - tagsW - 1
	if remaining < 20 {
		remaining = 20
	}
	nameW := remaining / 2
	payeeW := remaining - nameW
	if nameW < 10 {
		nameW = 10
	}
	if payeeW < 10 {
		payeeW = 10
	}
	m.table.SetColumns([]table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Payee", Width: payeeW},
		{Title: "Postings", Width: postingsW},
		{Title: "Tags", Width: tagsW},
	})
}

func (m TemplatesTab) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick(), FetchTemplates(m.client))
}

func (m TemplatesTab) capturesAllKeys() bool {
	return m.confirmDeleteID != ""
}

func (m TemplatesTab) KeyMap() help.KeyMap {
	if m.confirmDeleteID != "" {
		return DeleteConfirmKeyMap{}
	}
	if m.mode == templatesModeDetail {
		return TemplatesDetailKeyMap{}
	}
	return TemplatesListKeyMap{}
}

func (m TemplatesTab) Update(msg tea.Msg) (TemplatesTab, tea.Cmd) {
	switch msg := msg.(type) {
	case TemplatesMsg:
		if msg.Err != nil {
			m.SetError(msg.Err.Error())
		} else {
			m.setTemplates(msg.Templates)
		}
		return m, nil
	case DeleteTemplateMsg:
		m.confirmDeleteID = ""
		if msg.Err != nil {
			m.deleteErrMsg = msg.Err.Error()
		} else {
			m.deleteErrMsg = ""
			m.mode = templatesModeList
			m.panelBase = newPanelBase()
			return m, tea.Batch(m.spinner.Tick(), FetchTemplates(m.client))
		}
		return m, nil
	case tea.KeyMsg:
		if m.confirmDeleteID != "" {
			switch msg.String() {
			case "y":
				id := m.confirmDeleteID
				m.confirmDeleteID = ""
				return m, DeleteTemplateCmd(m.client, id)
			case "esc", "n":
				m.confirmDeleteID = ""
				m.deleteErrMsg = ""
			}
			return m, nil
		}

		if m.mode == templatesModeDetail {
			switch msg.String() {
			case "esc":
				m.mode = templatesModeList
			case "d":
				if tmpl := m.selectedTemplate(); tmpl != nil {
					m.confirmDeleteID = tmpl.Id
					m.deleteErrMsg = ""
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "r":
			m.panelBase = newPanelBase()
			return m, tea.Batch(m.spinner.Tick(), FetchTemplates(m.client))
		case "enter":
			if m.selectedTemplate() != nil {
				m.mode = templatesModeDetail
			}
			return m, nil
		case "d":
			if tmpl := m.selectedTemplate(); tmpl != nil {
				m.confirmDeleteID = tmpl.Id
				m.deleteErrMsg = ""
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

func (m TemplatesTab) View() string {
	if m.confirmDeleteID != "" {
		name := m.confirmDeleteID
		if tmpl := m.templateByID(m.confirmDeleteID); tmpl != nil {
			name = tmpl.Name
		}
		content := fmt.Sprintf("Delete template %q?\n\n[y] confirm  [esc] cancel", name)
		return RenderModal(m.width, m.height, "Confirm Delete", content, m.styles)
	}

	switch m.state {
	case stateLoading:
		return m.renderLoading()
	case stateError:
		return m.renderError(true)
	case stateLoaded:
		if len(m.templates) == 0 {
			return lipgloss.NewStyle().
				Width(m.width).Height(m.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No transaction templates found.")
		}
		if m.mode == templatesModeDetail {
			return m.detailView()
		}
		body := m.table.View()
		if m.deleteErrMsg != "" {
			errLine := m.styles.Error.Render("! " + m.deleteErrMsg)
			body = lipgloss.JoinVertical(lipgloss.Left, body, errLine)
		}
		return body
	}
	return ""
}

func (m *TemplatesTab) setTemplates(templates []*floatv1.TransactionTemplate) {
	m.templates = templates
	m.state = stateLoaded
	rows := make([]table.Row, len(templates))
	for i, tmpl := range templates {
		rows[i] = table.Row{
			tmpl.GetName(),
			tmpl.GetPayee(),
			fmt.Sprintf("%d", len(tmpl.GetPostings())),
			formatTemplateTags(tmpl.GetTags()),
		}
	}
	m.table.SetRows(rows)
}

func (m TemplatesTab) selectedTemplate() *floatv1.TransactionTemplate {
	if m.state != stateLoaded || len(m.templates) == 0 {
		return nil
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.templates) {
		return nil
	}
	return m.templates[idx]
}

func (m TemplatesTab) templateByID(id string) *floatv1.TransactionTemplate {
	for _, tmpl := range m.templates {
		if tmpl.GetId() == id {
			return tmpl
		}
	}
	return nil
}

func (m TemplatesTab) detailView() string {
	tmpl := m.selectedTemplate()
	if tmpl == nil {
		return ""
	}

	lines := []string{
		m.styles.Active.Render(tmpl.GetName()),
		"",
		fmt.Sprintf("Payee: %s", emptyDash(tmpl.GetPayee())),
		fmt.Sprintf("Note:  %s", emptyDash(tmpl.GetNote())),
		fmt.Sprintf("Tags:  %s", emptyDash(formatTemplateTags(tmpl.GetTags()))),
		"",
		m.styles.Help.Render("Postings"),
	}
	for _, posting := range tmpl.GetPostings() {
		amount := "auto"
		if posting.GetDefaultQuantity() != "" {
			amount = strings.TrimSpace(posting.GetDefaultQuantity() + " " + posting.GetCommodity())
		}
		line := fmt.Sprintf("  %-38s %s", posting.GetAccount(), amount)
		if posting.GetComment() != "" {
			line += "  ; " + posting.GetComment()
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", m.styles.Help.Render("esc back · d delete"))
	if m.deleteErrMsg != "" {
		lines = append(lines, m.styles.Error.Render("! "+m.deleteErrMsg))
	}
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(strings.Join(lines, "\n"))
}

func formatTemplateTags(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if tags[k] == "" {
			parts = append(parts, k)
		} else {
			parts = append(parts, k+"="+tags[k])
		}
	}
	return strings.Join(parts, " ")
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
