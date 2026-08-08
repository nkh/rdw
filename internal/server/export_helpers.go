package server

import (
	"github.com/nkh/rdw/internal/export"
	"github.com/nkh/rdw/internal/session"
)

func (s *Server) exportPane(id session.TargetID, outDir string) error {
	windows := []export.WindowExport{
		{Name: "pane", Panes: []export.PaneExport{{TargetID: id.String(), Lines: s.scrollbackFor(id)}}},
	}
	return export.Bundle(windows, outDir, id.String()+".md")
}

func (s *Server) exportWindow(win *session.WindowState, outDir string) error {
	var panes []export.PaneExport
	for _, p := range win.Panes {
		panes = append(panes, export.PaneExport{TargetID: p.TargetID.String(), Lines: s.scrollbackFor(p.TargetID)})
	}
	return export.Bundle([]export.WindowExport{{Name: win.Name, Panes: panes}}, outDir, win.Name+".md")
}

func (s *Server) exportAll(outDir string) error {
	var windows []export.WindowExport
	for _, win := range s.manager.Windows() {
		var panes []export.PaneExport
		for _, p := range win.Panes {
			panes = append(panes, export.PaneExport{TargetID: p.TargetID.String(), Lines: s.scrollbackFor(p.TargetID)})
		}
		windows = append(windows, export.WindowExport{Name: win.Name, Panes: panes})
	}
	return export.Bundle(windows, outDir, "session.md")
}

func (s *Server) scrollbackFor(id session.TargetID) []string {
	pl, ok := s.router.Get(id)
	if !ok {
		return nil
	}
	return pl.Scrollback().Lines()
}
