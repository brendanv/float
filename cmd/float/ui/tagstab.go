package ui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type tagsMode int

const (
	tagsModeList   tagsMode = iota
	tagsModeDetail tagsMode = iota
)

// TagsTab shows all user-visible tags and lets the user drill into transactions
// for a given tag.
type TagsTab struct {
	width  int
	height int
	client floatv1connect.LedgerServiceClient
	styles Styles
	mode   tagsMode
	list   tagsListPanel
	detail tagsDetailPanel
}

func NewTagsTab(client floatv1connect.LedgerServiceClient, st Styles) TagsTab {
	return TagsTab{
		client: client,
		styles: st,
		mode:   tagsModeList,
		list:   newTagsListPanel(st),
		detail: newTagsDetailPanel(st),
	}
}

func (m TagsTab) setStyles(st Styles) TagsTab {
	m.styles = st
	m.list.setStyles(st)
	m.detail.setStyles(st)
	return m
}

func (m TagsTab) SetSize(w, h int) TagsTab {
	m.width = w
	m.height = h
	m.list.SetSize(w, h)
	m.detail.SetSize(w, h)
	return m
}

func (m TagsTab) Init() tea.Cmd {
	return tea.Batch(m.list.spinner.Tick(), FetchTags(m.client))
}

func (m TagsTab) KeyMap() help.KeyMap {
	if m.mode == tagsModeDetail {
		return TagsDetailKeyMap{}
	}
	return TagsListKeyMap{}
}

func (m TagsTab) Update(msg tea.Msg) (TagsTab, tea.Cmd) {
	switch msg := msg.(type) {
	case TagsMsg:
		if msg.Err != nil {
			m.list.SetError(msg.Err.Error())
		} else {
			m.list.setTags(msg.Tags)
		}
		return m, nil

	case TagTransactionsMsg:
		if msg.Tag != m.detail.tag {
			return m, nil
		}
		if msg.Err != nil {
			m.detail.txPanel.SetError(msg.Err.Error())
		} else {
			m.detail.txPanel.SetTransactions(msg.Transactions)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case tagsModeList:
			switch msg.String() {
			case "r":
				m.list.panelBase = newPanelBase()
				return m, tea.Batch(m.list.spinner.Tick(), FetchTags(m.client))
			case "enter":
				tag := m.list.selected()
				if tag == "" {
					return m, nil
				}
				m.mode = tagsModeDetail
				m.detail = newTagsDetailPanel(m.styles)
				m.detail.SetSize(m.width, m.height)
				m.detail.tag = tag
				m.detail.txPanel.Focus()
				return m, tea.Batch(
					m.detail.txPanel.spinner.Tick(),
					FetchTagTransactions(m.client, tag),
				)
			default:
				cmd := m.list.Update(msg)
				return m, cmd
			}

		case tagsModeDetail:
			switch msg.String() {
			case "esc":
				m.mode = tagsModeList
				return m, nil
			case "r":
				m.detail.txPanel.panelBase = newPanelBase()
				return m, tea.Batch(
					m.detail.txPanel.spinner.Tick(),
					FetchTagTransactions(m.client, m.detail.tag),
				)
			default:
				cmd := m.detail.txPanel.Update(msg)
				return m, cmd
			}
		}

	default:
		switch m.mode {
		case tagsModeList:
			cmd := m.list.Update(msg)
			return m, cmd
		case tagsModeDetail:
			cmd := m.detail.txPanel.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m TagsTab) View() string {
	switch m.mode {
	case tagsModeList:
		return m.list.View()
	case tagsModeDetail:
		header := m.detail.renderHeader()
		return lipgloss.JoinVertical(lipgloss.Left, header, m.detail.txPanel.View())
	}
	return ""
}

// ── list panel ───────────────────────────────────────────────────────────────

type tagsListPanel struct {
	panelBase
	styles Styles
	tags   []string
	table  table.Model
}

func newTagsListTable(st Styles) table.Model {
	return table.New(
		table.WithColumns([]table.Column{
			{Title: "Tag", Width: 40},
		}),
		table.WithStyles(styledTableStyles(st)),
		table.WithFocused(true),
	)
}

func newTagsListPanel(st Styles) tagsListPanel {
	return tagsListPanel{
		styles:    st,
		panelBase: newPanelBase(),
		table:     newTagsListTable(st),
	}
}

func (p *tagsListPanel) setStyles(st Styles) {
	p.styles = st
	p.table.SetStyles(styledTableStyles(st))
}

func (p *tagsListPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.table.SetColumns([]table.Column{
		{Title: "Tag", Width: w - 1},
	})
	p.table.SetWidth(w)
	p.table.SetHeight(h)
}

func (p *tagsListPanel) setTags(tags []string) {
	p.tags = tags
	p.state = stateLoaded
	rows := make([]table.Row, len(tags))
	for i, tag := range tags {
		rows[i] = table.Row{tag}
	}
	p.table.SetRows(rows)
}

func (p *tagsListPanel) selected() string {
	if len(p.tags) == 0 {
		return ""
	}
	c := p.table.Cursor()
	if c < 0 || c >= len(p.tags) {
		return ""
	}
	return p.tags[c]
}

func (p *tagsListPanel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.state != stateLoaded {
			return nil
		}
		var cmd tea.Cmd
		p.table, cmd = p.table.Update(msg)
		return cmd
	}
	return p.handleSpinnerTick(msg)
}

func (p tagsListPanel) View() string {
	switch p.state {
	case stateLoading:
		return p.renderLoading()
	case stateError:
		return p.renderError(true)
	case stateLoaded:
		if len(p.tags) == 0 {
			return lipgloss.NewStyle().
				Width(p.width).Height(p.height).
				Align(lipgloss.Center, lipgloss.Center).
				Render("No tags found.")
		}
		return p.table.View()
	}
	return ""
}

// ── detail panel ─────────────────────────────────────────────────────────────

type tagsDetailPanel struct {
	width   int
	height  int
	styles  Styles
	tag     string
	txPanel TransactionsPanel
}

func newTagsDetailPanel(st Styles) tagsDetailPanel {
	return tagsDetailPanel{
		styles:  st,
		txPanel: newTransactionsPanel(st),
	}
}

func (p *tagsDetailPanel) setStyles(st Styles) {
	p.styles = st
	p.txPanel.setStyles(st)
}

func (p *tagsDetailPanel) SetSize(w, h int) {
	p.width = w
	p.height = h
	headerH := 2
	p.txPanel.SetSize(w, h-headerH)
}

func (p tagsDetailPanel) renderHeader() string {
	header := p.styles.TabInactive.Render("← esc") + "  Tag: " + p.tag
	return lipgloss.NewStyle().Width(p.width).Render(header)
}
