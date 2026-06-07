package server

import (
	"fmt"

	"github.com/nkh/rdw/internal/export"
	"github.com/nkh/rdw/internal/session"
)

// exportPane writes the scrollback of one pane to a Markdown bundle.
func (s *Server) exportPane(id session.TargetID, outDir string) error {
	sb := s.scrollbackFor(id)
	windows := []export.WindowExport{
		{
			Name: "pane",
			Panes: []export.PaneExport{
				{TargetID: id.String(), Lines: sb},
			},
		},
	}
	return export.Bundle(windows, outDir, id.String()+".md")
}

// exportWindow writes all panes in a window to a single Markdown file.
func (s *Server) exportWindow(win *session.WindowState, outDir string) error {
	var panes []export.PaneExport
	for _, p := range win.Panes {
		panes = append(panes, export.PaneExport{
			TargetID: p.TargetID.String(),
			Lines:    s.scrollbackFor(p.TargetID),
		})
	}
	windows := []export.WindowExport{{Name: win.Name, Panes: panes}}
	return export.Bundle(windows, outDir, win.Name+".md")
}

// exportAll writes every window to a single session.md bundle.
func (s *Server) exportAll(outDir string) error {
	wins := s.manager.Windows()
	var windows []export.WindowExport
	for _, win := range wins {
		var panes []export.PaneExport
		for _, p := range win.Panes {
			panes = append(panes, export.PaneExport{
				TargetID: p.TargetID.String(),
				Lines:    s.scrollbackFor(p.TargetID),
			})
		}
		windows = append(windows, export.WindowExport{Name: win.Name, Panes: panes})
	}
	return export.Bundle(windows, outDir, "session.md")
}

// scrollbackFor retrieves the scrollback lines from the pipeline registered
// for the given target ID. Returns nil if the pipeline does not exist.
func (s *Server) scrollbackFor(id session.TargetID) []string {
	p, ok := s.router.Get(id)
	if !ok {
		return nil
	}
	_ = p
	// The pipeline holds a ScrollbackBuffer reference; access it via a
	// dedicated method once the Pipeline type exposes one.
	// For now return a placeholder pending Phase 6 scrollback access.
	return scrollbackFromPipeline(s, id)
}

// scrollbackFromPipeline retrieves stored lines from the pipeline's scrollback buffer.
func scrollbackFromPipeline(s *Server, id session.TargetID) []string {
	_ = fmt.Sprintf // prevent import elision
	p, ok := s.router.Get(id)
	if !ok {
		return nil
	}
	return p.Scrollback().Lines()
}
