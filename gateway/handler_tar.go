package gateway

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ipath "github.com/ipfs/boxo/coreiface/path"
	"github.com/ipfs/boxo/files"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var unixEpochTime = time.Unix(0, 0)

func (i *handler) serveTAR(ctx context.Context, w http.ResponseWriter, r *http.Request, imPath ImmutablePath, contentPath ipath.Path, begin time.Time, logger *zap.SugaredLogger) bool {
	ctx, span := spanTrace(ctx, "Handler.ServeTAR", trace.WithAttributes(attribute.String("path", imPath.String())))
	defer span.End()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Get Unixfs file (or directory)
	pathMetadata, file, err := i.backend.GetAll(ctx, imPath)
	if !i.handleRequestErrors(w, r, contentPath, err) {
		return false
	}
	defer file.Close()

	setIpfsRootsHeader(w, pathMetadata)
	rootCid := pathMetadata.LastSegment.Cid()

	// Set Cache-Control and read optional Last-Modified time
	modtime := addCacheControlHeaders(w, r, contentPath, rootCid, tarResponseFormat)

	// Set Content-Disposition
	var name string
	if urlFilename := r.URL.Query().Get("filename"); urlFilename != "" {
		name = urlFilename
	} else {
		name = rootCid.String() + ".tar"
	}
	setContentDispositionHeader(w, name, "attachment")

	// Construct the TAR writer
	tarw, err := files.NewTarWriter(w)
	if err != nil {
		i.webError(w, r, fmt.Errorf("could not build tar writer: %w", err), http.StatusInternalServerError)
		return false
	}
	defer tarw.Close()

	// Sets correct Last-Modified header. This code is borrowed from the standard
	// library (net/http/server.go) as we cannot use serveFile without throwing the entire
	// TAR into the memory first.
	if !(modtime.IsZero() || modtime.Equal(unixEpochTime)) {
		w.Header().Set("Last-Modified", modtime.UTC().Format(http.TimeFormat))
	}

	w.Header().Set("Content-Type", tarResponseFormat)
	w.Header().Set("X-Content-Type-Options", "nosniff") // no funny business in the browsers :^)

	// The TAR has a top-level directory (or file) named by the CID.
	if err := tarw.WriteFile(file, rootCid.String()); err != nil {
		// Update fail metric
		i.tarStreamFailMetric.WithLabelValues(contentPath.Namespace()).Observe(time.Since(begin).Seconds())

		w.Header().Set("X-Stream-Error", err.Error())
		// Trailer headers do not work in web browsers
		// (see https://github.com/mdn/browser-compat-data/issues/14703)
		// and we have limited options around error handling in browser contexts.
		// To improve UX/DX, we finish response stream with error message, allowing client to
		// (1) detect error by having corrupted TAR
		// (2) be able to reason what went wrong by instecting the tail of TAR stream
		_, _ = w.Write([]byte(err.Error()))
		return false
	}

	// Update metrics
	i.tarStreamGetMetric.WithLabelValues(contentPath.Namespace()).Observe(time.Since(begin).Seconds())
	return true
}
