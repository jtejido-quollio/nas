package nasapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	client    client.Client
	namespace string
	webRoot   string
	logger    *log.Logger
	mux       *http.ServeMux
}

type apiError struct {
	Error string `json:"error"`
}

type overviewResponse struct {
	Pools       []nasv1.ZPool       `json:"pools"`
	Datasets    []nasv1.ZDataset    `json:"datasets"`
	Shares      []nasv1.NASShare    `json:"shares"`
	Directories []nasv1.NASDirectory `json:"directories"`
}

type createRequest[T any] struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Spec      T      `json:"spec"`
}

type secretRequest struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Data      map[string]string `json:"data"`
}

func NewServer(c client.Client, namespace, webRoot string, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(os.Stdout, "nas-api ", log.LstdFlags)
	}
	mux := http.NewServeMux()
	s := &Server{
		client:    c,
		namespace: namespace,
		webRoot:   webRoot,
		logger:    logger,
		mux:       mux,
	}

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/overview", s.handleOverview)
	mux.HandleFunc("/v1/zpools", s.handleZPools)
	mux.HandleFunc("/v1/zpools/", s.handleZPool)
	mux.HandleFunc("/v1/zdatasets", s.handleZDatasets)
	mux.HandleFunc("/v1/zdatasets/", s.handleZDataset)
	mux.HandleFunc("/v1/nasshares", s.handleNASShares)
	mux.HandleFunc("/v1/nasshares/", s.handleNASShare)
	mux.HandleFunc("/v1/nasdirectories", s.handleNASDirectories)
	mux.HandleFunc("/v1/nasdirectories/", s.handleNASDirectory)
	mux.HandleFunc("/v1/secrets", s.handleSecrets)
	mux.HandleFunc("/v1/secrets/", s.handleSecret)

	if webRoot != "" {
		fs := http.FileServer(http.Dir(webRoot))
		mux.Handle("/", fs)
	}

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.webRoot == "" && (r.URL.Path == "/" || r.URL.Path == "") {
		writeJSON(w, http.StatusOK, map[string]string{"message": "nas-api"})
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var pools nasv1.ZPoolList
	var datasets nasv1.ZDatasetList
	var shares nasv1.NASShareList
	var directories nasv1.NASDirectoryList

	if err := s.client.List(ctx, &pools, client.InNamespace(s.namespace)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.client.List(ctx, &datasets, client.InNamespace(s.namespace)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.client.List(ctx, &shares, client.InNamespace(s.namespace)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.client.List(ctx, &directories, client.InNamespace(s.namespace)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := overviewResponse{
		Pools:       pools.Items,
		Datasets:    datasets.Items,
		Shares:      shares.Items,
		Directories: directories.Items,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleZPools(w http.ResponseWriter, r *http.Request) {
	handleListOrCreate(s, w, r, func(ctx context.Context, ns string) (any, error) {
		var list nasv1.ZPoolList
		if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		return list.Items, nil
	}, func(ctx context.Context, req createRequest[nasv1.ZPoolSpec]) (any, error) {
		obj := nasv1.ZPool{
			TypeMeta: metav1.TypeMeta{APIVersion: "nas.io/v1alpha1", Kind: "ZPool"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: nsOrDefault(req.Namespace, s.namespace),
			},
			Spec: req.Spec,
		}
		return obj, upsertResource(ctx, s.client, &obj)
	})
}

func (s *Server) handleZPool(w http.ResponseWriter, r *http.Request) {
	s.handleGetOrDelete(w, r, "/v1/zpools/", func(ctx context.Context, name string) (any, error) {
		var obj nasv1.ZPool
		if err := s.client.Get(ctx, namespacedName(s.namespace, name), &obj); err != nil {
			return nil, err
		}
		return obj, nil
	}, func(ctx context.Context, name string) error {
		obj := &nasv1.ZPool{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.namespace}}
		return s.client.Delete(ctx, obj)
	})
}

func (s *Server) handleZDatasets(w http.ResponseWriter, r *http.Request) {
	handleListOrCreate(s, w, r, func(ctx context.Context, ns string) (any, error) {
		var list nasv1.ZDatasetList
		if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		return list.Items, nil
	}, func(ctx context.Context, req createRequest[nasv1.ZDatasetSpec]) (any, error) {
		obj := nasv1.ZDataset{
			TypeMeta: metav1.TypeMeta{APIVersion: "nas.io/v1alpha1", Kind: "ZDataset"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: nsOrDefault(req.Namespace, s.namespace),
			},
			Spec: req.Spec,
		}
		return obj, upsertResource(ctx, s.client, &obj)
	})
}

func (s *Server) handleZDataset(w http.ResponseWriter, r *http.Request) {
	s.handleGetOrDelete(w, r, "/v1/zdatasets/", func(ctx context.Context, name string) (any, error) {
		var obj nasv1.ZDataset
		if err := s.client.Get(ctx, namespacedName(s.namespace, name), &obj); err != nil {
			return nil, err
		}
		return obj, nil
	}, func(ctx context.Context, name string) error {
		obj := &nasv1.ZDataset{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.namespace}}
		return s.client.Delete(ctx, obj)
	})
}

func (s *Server) handleNASShares(w http.ResponseWriter, r *http.Request) {
	handleListOrCreate(s, w, r, func(ctx context.Context, ns string) (any, error) {
		var list nasv1.NASShareList
		if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		return list.Items, nil
	}, func(ctx context.Context, req createRequest[nasv1.NASShareSpec]) (any, error) {
		obj := nasv1.NASShare{
			TypeMeta: metav1.TypeMeta{APIVersion: "nas.io/v1alpha1", Kind: "NASShare"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: nsOrDefault(req.Namespace, s.namespace),
			},
			Spec: req.Spec,
		}
		return obj, upsertResource(ctx, s.client, &obj)
	})
}

func (s *Server) handleNASShare(w http.ResponseWriter, r *http.Request) {
	s.handleGetOrDelete(w, r, "/v1/nasshares/", func(ctx context.Context, name string) (any, error) {
		var obj nasv1.NASShare
		if err := s.client.Get(ctx, namespacedName(s.namespace, name), &obj); err != nil {
			return nil, err
		}
		return obj, nil
	}, func(ctx context.Context, name string) error {
		obj := &nasv1.NASShare{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.namespace}}
		return s.client.Delete(ctx, obj)
	})
}

func (s *Server) handleNASDirectories(w http.ResponseWriter, r *http.Request) {
	handleListOrCreate(s, w, r, func(ctx context.Context, ns string) (any, error) {
		var list nasv1.NASDirectoryList
		if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		return list.Items, nil
	}, func(ctx context.Context, req createRequest[nasv1.NASDirectorySpec]) (any, error) {
		obj := nasv1.NASDirectory{
			TypeMeta: metav1.TypeMeta{APIVersion: "nas.io/v1alpha1", Kind: "NASDirectory"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: nsOrDefault(req.Namespace, s.namespace),
			},
			Spec: req.Spec,
		}
		return obj, upsertResource(ctx, s.client, &obj)
	})
}

func (s *Server) handleNASDirectory(w http.ResponseWriter, r *http.Request) {
	s.handleGetOrDelete(w, r, "/v1/nasdirectories/", func(ctx context.Context, name string) (any, error) {
		var obj nasv1.NASDirectory
		if err := s.client.Get(ctx, namespacedName(s.namespace, name), &obj); err != nil {
			return nil, err
		}
		return obj, nil
	}, func(ctx context.Context, name string) error {
		obj := &nasv1.NASDirectory{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.namespace}}
		return s.client.Delete(ctx, obj)
	})
}

func (s *Server) handleSecrets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req secretRequest
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, "name required")
			return
		}
		if len(req.Data) == 0 {
			writeError(w, http.StatusBadRequest, "data required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		obj := corev1.Secret{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: nsOrDefault(req.Namespace, s.namespace),
			},
			Type:       corev1.SecretTypeOpaque,
			StringData: req.Data,
		}
		if err := upsertResource(ctx, s.client, &obj); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, obj)
	case http.MethodGet:
		writeError(w, http.StatusNotImplemented, "listing secrets is not supported")
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleSecret(w http.ResponseWriter, r *http.Request) {
	s.handleGetOrDelete(w, r, "/v1/secrets/", func(ctx context.Context, name string) (any, error) {
		return nil, errors.New("get secret is not supported")
	}, func(ctx context.Context, name string) error {
		return errors.New("delete secret is not supported")
	})
}

func handleListOrCreate[T any](s *Server, w http.ResponseWriter, r *http.Request, listFn func(context.Context, string) (any, error), createFn func(context.Context, createRequest[T]) (any, error)) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := listFn(ctx, s.namespace)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var req createRequest[T]
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, "name required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		obj, err := createFn(ctx, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, obj)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleGetOrDelete(w http.ResponseWriter, r *http.Request, pathPrefix string, getFn func(context.Context, string) (any, error), deleteFn func(context.Context, string) error) {
	name := strings.TrimPrefix(r.URL.Path, pathPrefix)
	name = strings.Trim(name, "/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		obj, err := getFn(ctx, name)
		if err != nil {
			if apiErrors.IsNotFound(err) {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, obj)
	case http.MethodDelete:
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := deleteFn(ctx, name); err != nil {
			if apiErrors.IsNotFound(err) {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) error {
	if r.Body == nil {
		return errors.New("request body required")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func nsOrDefault(ns, fallback string) string {
	ns = strings.TrimSpace(ns)
	if ns == "" {
		return fallback
	}
	return ns
}

func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}

func upsertResource(ctx context.Context, c client.Client, obj client.Object) error {
	key := namespacedName(obj.GetNamespace(), obj.GetName())
	current := obj.DeepCopyObject().(client.Object)
	if err := c.Get(ctx, key, current); err != nil {
		if apiErrors.IsNotFound(err) {
			return c.Create(ctx, obj)
		}
		return err
	}
	obj.SetResourceVersion(current.GetResourceVersion())
	return c.Update(ctx, obj)
}

func sanitizeFilePath(root, p string) string {
	clean := filepath.Clean("/" + p)
	return filepath.Join(root, strings.TrimPrefix(clean, "/"))
}
