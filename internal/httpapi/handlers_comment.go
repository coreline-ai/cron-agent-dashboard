package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) registerCommentRoutes(api chi.Router) {
	api.Get("/api/issues/{id}/comments", s.listComments)
	api.Post("/api/issues/{id}/comments", s.addComment)
	api.Delete("/api/comments/{id}", s.deleteComment)
}

func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListComments(r.Context(), chi.URLParam(r, "id"))
	respond(w, map[string]any{"comments": xs}, err, http.StatusOK)
}

func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := s.store.AddUserComment(r.Context(), chi.URLParam(r, "id"), req.Content)
	respond(w, res, err, http.StatusCreated)
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.store.DeleteComment(r.Context(), id)
	respond(w, map[string]any{"deleted": true, "id": id}, err, http.StatusOK)
}
