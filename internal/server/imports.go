package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/rudyh0/afterfeed/internal/importer"
)

func (s *Server) importRoutes() {
	m := s.mux
	m.HandleFunc("GET /api/import-folder", s.authed(s.importFolder, false))
	m.HandleFunc("GET /api/uploads", s.authed(s.listUploads, false))
	m.HandleFunc("POST /api/uploads", s.authed(s.createUpload, false))
	m.HandleFunc("PUT /api/uploads/{id}", s.authed(s.writeUpload, false))
	m.HandleFunc("DELETE /api/uploads/{id}", s.authed(s.deleteUpload, false))
	m.HandleFunc("GET /api/imports", s.authed(s.listImports, false))
	m.HandleFunc("POST /api/imports", s.authed(s.createImport, false))
	m.HandleFunc("GET /api/imports/{id}", s.authed(s.getImport, false))
	m.HandleFunc("POST /api/imports/{id}/files", s.authed(s.addImportFiles, false))
	m.HandleFunc("POST /api/imports/{id}/continue", s.authed(s.continueImport, false))
	m.HandleFunc("POST /api/imports/{id}/retry", s.authed(s.retryImport, false))
	m.HandleFunc("DELETE /api/imports/{id}", s.authed(s.cancelImport, false))
}

// importError turns importer errors into API errors with stable codes.
func importError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound()
	}
	for _, e := range []error{importer.ErrOffset, importer.ErrUploadTooBig, importer.ErrFileGone, importer.ErrUnknownPlatform} {
		if errors.Is(err, e) {
			return errBadRequest(e.Error(), err.Error())
		}
	}
	if errors.Is(err, importer.ErrNoSpace) {
		return &apiError{Status: http.StatusInsufficientStorage, Code: importer.ErrNoSpace.Error(), Message: "not enough free space"}
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "import.") || strings.HasPrefix(msg, "upload.") {
		code, _, _ := strings.Cut(msg, ":")
		return errBadRequest(code, msg)
	}
	return err
}

func (s *Server) importFolder(w http.ResponseWriter, r *http.Request) error {
	entries, err := s.deps.Imports.ImportFolder(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
	return nil
}

func (s *Server) listUploads(w http.ResponseWriter, r *http.Request) error {
	list, err := s.deps.Imports.ListUploads(r.Context(), r.URL.Query().Get("platform"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"uploads": list, "chunk_size": importer.ChunkSize})
	return nil
}

func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		Modified int64  `json:"modified"`
		Platform string `json:"platform"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	u, err := s.deps.Imports.CreateUpload(r.Context(), req.Name, req.Size, req.Modified, req.Platform)
	if err != nil {
		return importError(err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"upload": u, "chunk_size": importer.ChunkSize})
	return nil
}

func (s *Server) writeUpload(w http.ResponseWriter, r *http.Request) error {
	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil || offset < 0 {
		return errBadRequest("upload.offset", "invalid offset")
	}
	body := http.MaxBytesReader(w, r.Body, importer.ChunkSize+1)
	u, err := s.deps.Imports.WriteUpload(r.Context(), r.PathValue("id"), offset, body)
	if errors.Is(err, importer.ErrOffset) && u != nil {
		// Tell the client where to continue.
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  apiError{Code: "upload.offset", Message: "offset does not match"},
			"upload": u,
		})
		return nil
	}
	if err != nil {
		return importError(err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"upload": u})
	return nil
}

func (s *Server) deleteUpload(w http.ResponseWriter, r *http.Request) error {
	if err := s.deps.Imports.DeleteUpload(r.Context(), r.PathValue("id")); err != nil {
		return importError(err)
	}
	writeJSON(w, http.StatusOK, map[string]any{})
	return nil
}

func (s *Server) listImports(w http.ResponseWriter, r *http.Request) error {
	list, err := s.deps.Imports.List(r.Context(), 50)
	if err != nil {
		return err
	}
	if list == nil {
		list = []*importer.Status{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"imports":       list,
		"platforms":     s.deps.Imports.Platforms(),
		"previews_left": s.deps.Imports.PreviewsLeft(),
	})
	return nil
}

type importFilesRequest struct {
	Platform string   `json:"platform"`
	Uploads  []string `json:"uploads"`
	Files    []string `json:"files"`
}

func (req importFilesRequest) refs() []importer.FileRef {
	var refs []importer.FileRef
	for _, id := range req.Uploads {
		refs = append(refs, importer.FileRef{Kind: "upload", ID: id})
	}
	for _, p := range req.Files {
		refs = append(refs, importer.FileRef{Kind: "folder", Path: p})
	}
	return refs
}

func (s *Server) createImport(w http.ResponseWriter, r *http.Request) error {
	var req importFilesRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	id, err := s.deps.Imports.Create(r.Context(), req.Platform, req.refs())
	if err != nil {
		return importError(err)
	}
	return s.writeImport(w, r, id)
}

func (s *Server) writeImport(w http.ResponseWriter, r *http.Request, id int64) error {
	st, err := s.deps.Imports.Get(r.Context(), id)
	if err != nil {
		return importError(err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"import": st})
	return nil
}

func importID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound()
	}
	return id, nil
}

func (s *Server) getImport(w http.ResponseWriter, r *http.Request) error {
	id, err := importID(r)
	if err != nil {
		return err
	}
	return s.writeImport(w, r, id)
}

func (s *Server) addImportFiles(w http.ResponseWriter, r *http.Request) error {
	id, err := importID(r)
	if err != nil {
		return err
	}
	var req importFilesRequest
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if err := s.deps.Imports.AddFiles(r.Context(), id, req.refs()); err != nil {
		return importError(err)
	}
	return s.writeImport(w, r, id)
}

func (s *Server) continueImport(w http.ResponseWriter, r *http.Request) error {
	id, err := importID(r)
	if err != nil {
		return err
	}
	if err := s.deps.Imports.Continue(r.Context(), id); err != nil {
		return importError(err)
	}
	return s.writeImport(w, r, id)
}

func (s *Server) retryImport(w http.ResponseWriter, r *http.Request) error {
	id, err := importID(r)
	if err != nil {
		return err
	}
	if err := s.deps.Imports.Retry(r.Context(), id); err != nil {
		return importError(err)
	}
	return s.writeImport(w, r, id)
}

func (s *Server) cancelImport(w http.ResponseWriter, r *http.Request) error {
	id, err := importID(r)
	if err != nil {
		return err
	}
	if err := s.deps.Imports.Cancel(r.Context(), id); err != nil {
		return importError(err)
	}
	return s.writeImport(w, r, id)
}
