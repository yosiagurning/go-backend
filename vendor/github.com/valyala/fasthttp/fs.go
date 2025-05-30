package fasthttp

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/valyala/bytebufferpool"
)

// ServeFileBytesUncompressed returns HTTP response containing file contents
// from the given path.
//
// Directory contents is returned if path points to directory.
//
// ServeFileBytes may be used for saving network traffic when serving files
// with good compression ratio.
//
// See also RequestCtx.SendFileBytes.
//
// WARNING: do not pass any user supplied paths to this function!
// WARNING: if path is based on user input users will be able to request
// any file on your filesystem! Use fasthttp.FS with a sane Root instead.
func ServeFileBytesUncompressed(ctx *RequestCtx, path []byte) {
	ServeFileUncompressed(ctx, b2s(path))
}

// ServeFileUncompressed returns HTTP response containing file contents
// from the given path.
//
// Directory contents is returned if path points to directory.
//
// ServeFile may be used for saving network traffic when serving files
// with good compression ratio.
//
// See also RequestCtx.SendFile.
//
// WARNING: do not pass any user supplied paths to this function!
// WARNING: if path is based on user input users will be able to request
// any file on your filesystem! Use fasthttp.FS with a sane Root instead.
func ServeFileUncompressed(ctx *RequestCtx, path string) {
	ctx.Request.Header.DelBytes(strAcceptEncoding)
	ServeFile(ctx, path)
}

// ServeFileBytes returns HTTP response containing compressed file contents
// from the given path.
//
// HTTP response may contain uncompressed file contents in the following cases:
//
//   - Missing 'Accept-Encoding: gzip' request header.
//   - No write access to directory containing the file.
//
// Directory contents is returned if path points to directory.
//
// Use ServeFileBytesUncompressed is you don't need serving compressed
// file contents.
//
// See also RequestCtx.SendFileBytes.
//
// WARNING: do not pass any user supplied paths to this function!
// WARNING: if path is based on user input users will be able to request
// any file on your filesystem! Use fasthttp.FS with a sane Root instead.
func ServeFileBytes(ctx *RequestCtx, path []byte) {
	ServeFile(ctx, b2s(path))
}

// ServeFile returns HTTP response containing compressed file contents
// from the given path.
//
// HTTP response may contain uncompressed file contents in the following cases:
//
//   - Missing 'Accept-Encoding: gzip' request header.
//   - No write access to directory containing the file.
//
// Directory contents is returned if path points to directory.
//
// Use ServeFileUncompressed is you don't need serving compressed file contents.
//
// See also RequestCtx.SendFile.
//
// WARNING: do not pass any user supplied paths to this function!
// WARNING: if path is based on user input users will be able to request
// any file on your filesystem! Use fasthttp.FS with a sane Root instead.
func ServeFile(ctx *RequestCtx, path string) {
	rootFSOnce.Do(func() {
		rootFSHandler = rootFS.NewRequestHandler()
	})

	if path == "" || !filepath.IsAbs(path) {
		// extend relative path to absolute path
		hasTrailingSlash := path != "" && (path[len(path)-1] == '/' || path[len(path)-1] == '\\')

		var err error
		path = filepath.FromSlash(path)
		if path, err = filepath.Abs(path); err != nil {
			ctx.Logger().Printf("cannot resolve path %q to absolute file path: %v", path, err)
			ctx.Error("Internal Server Error", StatusInternalServerError)
			return
		}
		if hasTrailingSlash {
			path += "/"
		}
	}

	// convert the path to forward slashes regardless the OS in order to set the URI properly
	// the handler will convert back to OS path separator before opening the file
	path = filepath.ToSlash(path)

	ctx.Request.SetRequestURI(path)
	rootFSHandler(ctx)
}

var (
	rootFSOnce sync.Once
	rootFS     = &FS{
		Root:               "",
		AllowEmptyRoot:     true,
		GenerateIndexPages: true,
		Compress:           true,
		CompressBrotli:     true,
		CompressZstd:       true,
		AcceptByteRange:    true,
	}
	rootFSHandler RequestHandler
)

// ServeFS returns HTTP response containing compressed file contents from the given fs.FS's path.
//
// HTTP response may contain uncompressed file contents in the following cases:
//
//   - Missing 'Accept-Encoding: gzip' request header.
//   - No write access to directory containing the file.
//
// Directory contents is returned if path points to directory.
//
// See also ServeFile.
func ServeFS(ctx *RequestCtx, filesystem fs.FS, path string) {
	f := &FS{
		FS:                 filesystem,
		Root:               "",
		AllowEmptyRoot:     true,
		GenerateIndexPages: true,
		Compress:           true,
		CompressBrotli:     true,
		CompressZstd:       true,
		AcceptByteRange:    true,
	}
	handler := f.NewRequestHandler()

	ctx.Request.SetRequestURI(path)
	handler(ctx)
}

// PathRewriteFunc must return new request path based on arbitrary ctx
// info such as ctx.Path().
//
// Path rewriter is used in FS for translating the current request
// to the local filesystem path relative to FS.Root.
//
// The returned path must not contain '/../' substrings due to security reasons,
// since such paths may refer files outside FS.Root.
//
// The returned path may refer to ctx members. For example, ctx.Path().
type PathRewriteFunc func(ctx *RequestCtx) []byte

// NewVHostPathRewriter returns path rewriter, which strips slashesCount
// leading slashes from the path and prepends the path with request's host,
// thus simplifying virtual hosting for static files.
//
// Examples:
//
//   - host=foobar.com, slashesCount=0, original path="/foo/bar".
//     Resulting path: "/foobar.com/foo/bar"
//
//   - host=img.aaa.com, slashesCount=1, original path="/images/123/456.jpg"
//     Resulting path: "/img.aaa.com/123/456.jpg"
func NewVHostPathRewriter(slashesCount int) PathRewriteFunc {
	return func(ctx *RequestCtx) []byte {
		path := stripLeadingSlashes(ctx.Path(), slashesCount)
		host := ctx.Host()
		if n := bytes.IndexByte(host, '/'); n >= 0 {
			host = nil
		}
		if len(host) == 0 {
			host = strInvalidHost
		}
		b := bytebufferpool.Get()
		b.B = append(b.B, '/')
		b.B = append(b.B, host...)
		b.B = append(b.B, path...)
		ctx.URI().SetPathBytes(b.B)
		bytebufferpool.Put(b)

		return ctx.Path()
	}
}

var strInvalidHost = []byte("invalid-host")

// NewPathSlashesStripper returns path rewriter, which strips slashesCount
// leading slashes from the path.
//
// Examples:
//
//   - slashesCount = 0, original path: "/foo/bar", result: "/foo/bar"
//   - slashesCount = 1, original path: "/foo/bar", result: "/bar"
//   - slashesCount = 2, original path: "/foo/bar", result: ""
//
// The returned path rewriter may be used as FS.PathRewrite .
func NewPathSlashesStripper(slashesCount int) PathRewriteFunc {
	return func(ctx *RequestCtx) []byte {
		return stripLeadingSlashes(ctx.Path(), slashesCount)
	}
}

// NewPathPrefixStripper returns path rewriter, which removes prefixSize bytes
// from the path prefix.
//
// Examples:
//
//   - prefixSize = 0, original path: "/foo/bar", result: "/foo/bar"
//   - prefixSize = 3, original path: "/foo/bar", result: "o/bar"
//   - prefixSize = 7, original path: "/foo/bar", result: "r"
//
// The returned path rewriter may be used as FS.PathRewrite .
func NewPathPrefixStripper(prefixSize int) PathRewriteFunc {
	return func(ctx *RequestCtx) []byte {
		path := ctx.Path()
		if len(path) >= prefixSize {
			path = path[prefixSize:]
		}
		return path
	}
}

// FS represents settings for request handler serving static files
// from the local filesystem.
//
// It is prohibited copying FS values. Create new values instead.
type FS struct {
	noCopy noCopy

	// FS is filesystem to serve files from. eg: embed.FS os.DirFS
	FS fs.FS

	// Path rewriting function.
	//
	// By default request path is not modified.
	PathRewrite PathRewriteFunc

	// PathNotFound fires when file is not found in filesystem
	// this functions tries to replace "Cannot open requested path"
	// server response giving to the programmer the control of server flow.
	//
	// By default PathNotFound returns
	// "Cannot open requested path"
	PathNotFound RequestHandler

	// Suffixes list to add to compressedFileSuffix depending on encoding
	//
	// This value has sense only if Compress is set.
	//
	// FSCompressedFileSuffixes is used by default.
	CompressedFileSuffixes map[string]string

	// If CleanStop is set, the channel can be closed to stop the cleanup handlers
	// for the FS RequestHandlers created with NewRequestHandler.
	// NEVER close this channel while the handler is still being used!
	CleanStop chan struct{}

	h RequestHandler

	// Path to the root directory to serve files from.
	Root string

	// Path to the compressed root directory to serve files from. If this value
	// is empty, Root is used.
	CompressRoot string

	// Suffix to add to the name of cached compressed file.
	//
	// This value has sense only if Compress is set.
	//
	// FSCompressedFileSuffix is used by default.
	CompressedFileSuffix string

	// List of index file names to try opening during directory access.
	//
	// For example:
	//
	//     * index.html
	//     * index.htm
	//     * my-super-index.xml
	//
	// By default the list is empty.
	IndexNames []string

	// Expiration duration for inactive file handlers.
	//
	// FSHandlerCacheDuration is used by default.
	CacheDuration time.Duration

	once sync.Once

	// AllowEmptyRoot controls what happens when Root is empty. When false (default) it will default to the
	// current working directory. An empty root is mostly useful when you want to use absolute paths
	// on windows that are on different filesystems. On linux setting your Root to "/" already allows you to use
	// absolute paths on any filesystem.
	AllowEmptyRoot bool

	// Uses brotli encoding and fallbacks to zstd or gzip in responses if set to true, uses zstd or gzip if set to false.
	//
	// This value has sense only if Compress is set.
	//
	// Brotli encoding is disabled by default.
	CompressBrotli bool

	// Uses zstd encoding and fallbacks to gzip in responses if set to true, uses gzip if set to false.
	//
	// This value has sense only if Compress is set.
	//
	// zstd encoding is disabled by default.
	CompressZstd bool

	// Index pages for directories without files matching IndexNames
	// are automatically generated if set.
	//
	// Directory index generation may be quite slow for directories
	// with many files (more than 1K), so it is discouraged enabling
	// index pages' generation for such directories.
	//
	// By default index pages aren't generated.
	GenerateIndexPages bool

	// Transparently compresses responses if set to true.
	//
	// The server tries minimizing CPU usage by caching compressed files.
	// It adds CompressedFileSuffix suffix to the original file name and
	// tries saving the resulting compressed file under the new file name.
	// So it is advisable to give the server write access to Root
	// and to all inner folders in order to minimize CPU usage when serving
	// compressed responses.
	//
	// Transparent compression is disabled by default.
	Compress bool

	// Enables byte range requests if set to true.
	//
	// Byte range requests are disabled by default.
	AcceptByteRange bool

	// SkipCache if true, will cache no file handler.
	//
	// By default is false.
	SkipCache bool
}

// FSCompressedFileSuffix is the suffix FS adds to the original file names
// when trying to store compressed file under the new file name.
// See FS.Compress for details.
const FSCompressedFileSuffix = ".fasthttp.gz"

// FSCompressedFileSuffixes is the suffixes FS adds to the original file names depending on encoding
// when trying to store compressed file under the new file name.
// See FS.Compress for details.
var FSCompressedFileSuffixes = map[string]string{
	"gzip": ".fasthttp.gz",
	"br":   ".fasthttp.br",
	"zstd": ".fasthttp.zst",
}

// FSHandlerCacheDuration is the default expiration duration for inactive
// file handlers opened by FS.
const FSHandlerCacheDuration = 10 * time.Second

// FSHandler returns request handler serving static files from
// the given root folder.
//
// stripSlashes indicates how many leading slashes must be stripped
// from requested path before searching requested file in the root folder.
// Examples:
//
//   - stripSlashes = 0, original path: "/foo/bar", result: "/foo/bar"
//   - stripSlashes = 1, original path: "/foo/bar", result: "/bar"
//   - stripSlashes = 2, original path: "/foo/bar", result: ""
//
// The returned request handler automatically generates index pages
// for directories without index.html.
//
// The returned handler caches requested file handles
// for FSHandlerCacheDuration.
// Make sure your program has enough 'max open files' limit aka
// 'ulimit -n' if root folder contains many files.
//
// Do not create multiple request handler instances for the same
// (root, stripSlashes) arguments - just reuse a single instance.
// Otherwise goroutine leak will occur.
func FSHandler(root string, stripSlashes int) RequestHandler {
	fs := &FS{
		Root:               root,
		IndexNames:         []string{"index.html"},
		GenerateIndexPages: true,
		AcceptByteRange:    true,
	}
	if stripSlashes > 0 {
		fs.PathRewrite = NewPathSlashesStripper(stripSlashes)
	}
	return fs.NewRequestHandler()
}

// NewRequestHandler returns new request handler with the given FS settings.
//
// The returned handler caches requested file handles
// for FS.CacheDuration.
// Make sure your program has enough 'max open files' limit aka
// 'ulimit -n' if FS.Root folder contains many files.
//
// Do not create multiple request handlers from a single FS instance -
// just reuse a single request handler.
func (fs *FS) NewRequestHandler() RequestHandler {
	fs.once.Do(fs.initRequestHandler)
	return fs.h
}

func (fs *FS) normalizeRoot(root string) string {
	// fs.FS uses relative paths, that paths are slash-separated on all systems, even Windows.
	if fs.FS == nil {
		// Serve files from the current working directory if Root is empty or if Root is a relative path.
		if (!fs.AllowEmptyRoot && root == "") || (root != "" && !filepath.IsAbs(root)) {
			path, err := os.Getwd()
			if err != nil {
				path = "."
			}
			root = path + "/" + root
		}

		// convert the root directory slashes to the native format
		root = filepath.FromSlash(root)
	}

	// strip trailing slashes from the root path
	for root != "" && root[len(root)-1] == os.PathSeparator {
		root = root[:len(root)-1]
	}
	return root
}

func (fs *FS) initRequestHandler() {
	root := fs.normalizeRoot(fs.Root)

	compressRoot := fs.CompressRoot
	if compressRoot == "" {
		compressRoot = root
	} else {
		compressRoot = fs.normalizeRoot(compressRoot)
	}

	compressedFileSuffixes := fs.CompressedFileSuffixes
	if compressedFileSuffixes["br"] == "" || compressedFileSuffixes["gzip"] == "" ||
		compressedFileSuffixes["zstd"] == "" || compressedFileSuffixes["br"] == compressedFileSuffixes["gzip"] ||
		compressedFileSuffixes["br"] == compressedFileSuffixes["zstd"] ||
		compressedFileSuffixes["gzip"] == compressedFileSuffixes["zstd"] {
		// Copy global map
		compressedFileSuffixes = make(map[string]string, len(FSCompressedFileSuffixes))
		for k, v := range FSCompressedFileSuffixes {
			compressedFileSuffixes[k] = v
		}
	}

	if fs.CompressedFileSuffix != "" {
		compressedFileSuffixes["gzip"] = fs.CompressedFileSuffix
		compressedFileSuffixes["br"] = FSCompressedFileSuffixes["br"]
		compressedFileSuffixes["zstd"] = FSCompressedFileSuffixes["zstd"]
	}

	h := &fsHandler{
		filesystem:             fs.FS,
		root:                   root,
		indexNames:             fs.IndexNames,
		pathRewrite:            fs.PathRewrite,
		generateIndexPages:     fs.GenerateIndexPages,
		compress:               fs.Compress,
		compressBrotli:         fs.CompressBrotli,
		compressZstd:           fs.CompressZstd,
		compressRoot:           compressRoot,
		pathNotFound:           fs.PathNotFound,
		acceptByteRange:        fs.AcceptByteRange,
		compressedFileSuffixes: compressedFileSuffixes,
	}

	h.cacheManager = newCacheManager(fs)

	if h.filesystem == nil {
		h.filesystem = &osFS{} // It provides os.Open and os.Stat
	}

	fs.h = h.handleRequest
}

type fsHandler struct {
	smallFileReaderPool sync.Pool
	filesystem          fs.FS

	cacheManager cacheManager

	pathRewrite            PathRewriteFunc
	pathNotFound           RequestHandler
	compressedFileSuffixes map[string]string

	root               string
	compressRoot       string
	indexNames         []string
	generateIndexPages bool
	compress           bool
	compressBrotli     bool
	compressZstd       bool
	acceptByteRange    bool
}

type fsFile struct {
	lastModified time.Time

	t               time.Time
	f               fs.File
	h               *fsHandler
	filename        string // fs.FileInfo.Name() return filename, isn't filepath.
	contentType     string
	dirIndex        []byte
	lastModifiedStr []byte

	bigFiles      []*bigFileReader
	contentLength int
	readersCount  int

	bigFilesLock sync.Mutex
	compressed   bool
}

func (ff *fsFile) NewReader() (io.Reader, error) {
	if ff.isBig() {
		r, err := ff.bigFileReader()
		if err != nil {
			ff.decReadersCount()
		}
		return r, err
	}
	return ff.smallFileReader()
}

func (ff *fsFile) smallFileReader() (io.Reader, error) {
	v := ff.h.smallFileReaderPool.Get()
	if v == nil {
		v = &fsSmallFileReader{}
	}
	r := v.(*fsSmallFileReader)
	r.ff = ff
	r.endPos = ff.contentLength
	if r.startPos > 0 {
		return nil, errors.New("bug: fsSmallFileReader with non-nil startPos found in the pool")
	}
	return r, nil
}

// Files bigger than this size are sent with sendfile.
const maxSmallFileSize = 2 * 4096

func (ff *fsFile) isBig() bool {
	if _, ok := ff.h.filesystem.(*osFS); !ok { // fs.FS only uses bigFileReader, memory cache uses fsSmallFileReader
		return ff.f != nil
	}
	return ff.contentLength > maxSmallFileSize && len(ff.dirIndex) == 0
}

func (ff *fsFile) bigFileReader() (io.Reader, error) {
	if ff.f == nil {
		return nil, errors.New("bug: ff.f must be non-nil in bigFileReader")
	}

	var r io.Reader

	ff.bigFilesLock.Lock()
	n := len(ff.bigFiles)
	if n > 0 {
		r = ff.bigFiles[n-1]
		ff.bigFiles = ff.bigFiles[:n-1]
	}
	ff.bigFilesLock.Unlock()

	if r != nil {
		return r, nil
	}

	f, err := ff.h.filesystem.Open(ff.filename)
	if err != nil {
		return nil, fmt.Errorf("cannot open already opened file: %w", err)
	}
	return &bigFileReader{
		f:  f,
		ff: ff,
		r:  f,
	}, nil
}

func (ff *fsFile) Release() {
	if ff.f != nil {
		_ = ff.f.Close()

		if ff.isBig() {
			ff.bigFilesLock.Lock()
			for _, r := range ff.bigFiles {
				_ = r.f.Close()
			}
			ff.bigFilesLock.Unlock()
		}
	}
}

func (ff *fsFile) decReadersCount() {
	ff.h.cacheManager.WithLock(func() {
		ff.readersCount--
		if ff.readersCount < 0 {
			ff.readersCount = 0
		}
	})
}

// bigFileReader attempts to trigger sendfile
// for sending big files over the wire.
type bigFileReader struct {
	f  fs.File
	ff *fsFile
	r  io.Reader
	lr io.LimitedReader
}

func (r *bigFileReader) UpdateByteRange(startPos, endPos int) error {
	seeker, ok := r.f.(io.Seeker)
	if !ok {
		return errors.New("must implement io.Seeker")
	}
	if _, err := seeker.Seek(int64(startPos), io.SeekStart); err != nil {
		return err
	}
	r.r = &r.lr
	r.lr.R = r.f
	r.lr.N = int64(endPos - startPos + 1)
	return nil
}

func (r *bigFileReader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

func (r *bigFileReader) WriteTo(w io.Writer) (int64, error) {
	if rf, ok := w.(io.ReaderFrom); ok {
		// fast path. Send file must be triggered
		return rf.ReadFrom(r.r)
	}

	// slow path
	return copyZeroAlloc(w, r.r)
}

func (r *bigFileReader) Close() error {
	r.r = r.f
	seeker, ok := r.f.(io.Seeker)
	if !ok {
		_ = r.f.Close()
		return errors.New("must implement io.Seeker")
	}
	n, err := seeker.Seek(0, io.SeekStart)
	if err == nil {
		if n == 0 {
			ff := r.ff
			ff.bigFilesLock.Lock()
			ff.bigFiles = append(ff.bigFiles, r)
			ff.bigFilesLock.Unlock()
		} else {
			_ = r.f.Close()
			err = errors.New("bug: File.Seek(0, io.SeekStart) returned (non-zero, nil)")
		}
	} else {
		_ = r.f.Close()
	}
	r.ff.decReadersCount()
	return err
}

type fsSmallFileReader struct {
	ff       *fsFile
	startPos int
	endPos   int
}

func (r *fsSmallFileReader) Close() error {
	ff := r.ff
	ff.decReadersCount()
	r.ff = nil
	r.startPos = 0
	r.endPos = 0
	ff.h.smallFileReaderPool.Put(r)
	return nil
}

func (r *fsSmallFileReader) UpdateByteRange(startPos, endPos int) error {
	r.startPos = startPos
	r.endPos = endPos + 1
	return nil
}

func (r *fsSmallFileReader) Read(p []byte) (int, error) {
	tailLen := r.endPos - r.startPos
	if tailLen <= 0 {
		return 0, io.EOF
	}
	if len(p) > tailLen {
		p = p[:tailLen]
	}

	ff := r.ff
	if ff.f != nil {
		ra, ok := ff.f.(io.ReaderAt)
		if !ok {
			return 0, errors.New("must implement io.ReaderAt")
		}
		n, err := ra.ReadAt(p, int64(r.startPos))
		r.startPos += n
		return n, err
	}

	n := copy(p, ff.dirIndex[r.startPos:])
	r.startPos += n
	return n, nil
}

func (r *fsSmallFileReader) WriteTo(w io.Writer) (int64, error) {
	ff := r.ff

	var n int
	var err error
	if ff.f == nil {
		n, err = w.Write(ff.dirIndex[r.startPos:r.endPos])
		return int64(n), err
	}

	if rf, ok := w.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}

	curPos := r.startPos
	bufv := copyBufPool.Get()
	buf := bufv.([]byte)
	for err == nil {
		tailLen := r.endPos - curPos
		if tailLen <= 0 {
			break
		}
		if len(buf) > tailLen {
			buf = buf[:tailLen]
		}
		ra, ok := ff.f.(io.ReaderAt)
		if !ok {
			return 0, errors.New("must implement io.ReaderAt")
		}
		n, err = ra.ReadAt(buf, int64(curPos))
		nw, errw := w.Write(buf[:n])
		curPos += nw
		if errw == nil && nw != n {
			errw = errors.New("bug: Write(p) returned (n, nil), where n != len(p)")
		}
		if err == nil {
			err = errw
		}
	}
	copyBufPool.Put(bufv)

	if err == io.EOF {
		err = nil
	}
	return int64(curPos - r.startPos), err
}

type cacheManager interface {
	WithLock(work func())
	GetFileFromCache(cacheKind CacheKind, path string) (*fsFile, bool)
	SetFileToCache(cacheKind CacheKind, path string, ff *fsFile) *fsFile
}

var (
	_ cacheManager = (*inMemoryCacheManager)(nil)
	_ cacheManager = (*noopCacheManager)(nil)
)

type CacheKind uint8

const (
	defaultCacheKind CacheKind = iota
	brotliCacheKind
	gzipCacheKind
	zstdCacheKind
)

func newCacheManager(fs *FS) cacheManager {
	if fs.SkipCache {
		return &noopCacheManager{}
	}

	cacheDuration := fs.CacheDuration
	if cacheDuration <= 0 {
		cacheDuration = FSHandlerCacheDuration
	}

	instance := &inMemoryCacheManager{
		cacheDuration: cacheDuration,
		cache:         make(map[string]*fsFile),
		cacheBrotli:   make(map[string]*fsFile),
		cacheGzip:     make(map[string]*fsFile),
		cacheZstd:     make(map[string]*fsFile),
	}

	go instance.handleCleanCache(fs.CleanStop)

	return instance
}

type noopCacheManager struct {
	cacheLock sync.Mutex
}

func (n *noopCacheManager) WithLock(work func()) {
	n.cacheLock.Lock()

	work()

	n.cacheLock.Unlock()
}

func (*noopCacheManager) GetFileFromCache(cacheKind CacheKind, path string) (*fsFile, bool) {
	return nil, false
}

func (*noopCacheManager) SetFileToCache(cacheKind CacheKind, path string, ff *fsFile) *fsFile {
	return ff
}

type inMemoryCacheManager struct {
	cache         map[string]*fsFile
	cacheBrotli   map[string]*fsFile
	cacheGzip     map[string]*fsFile
	cacheZstd     map[string]*fsFile
	cacheDuration time.Duration
	cacheLock     sync.Mutex
}

func (cm *inMemoryCacheManager) WithLock(work func()) {
	cm.cacheLock.Lock()

	work()

	cm.cacheLock.Unlock()
}

func (cm *inMemoryCacheManager) getFsCache(cacheKind CacheKind) map[string]*fsFile {
	fileCache := cm.cache
	switch cacheKind {
	case brotliCacheKind:
		fileCache = cm.cacheBrotli
	case gzipCacheKind:
		fileCache = cm.cacheGzip
	case zstdCacheKind:
		fileCache = cm.cacheZstd
	}

	return fileCache
}

func (cm *inMemoryCacheManager) GetFileFromCache(cacheKind CacheKind, path string) (*fsFile, bool) {
	fileCache := cm.getFsCache(cacheKind)

	cm.cacheLock.Lock()
	ff, ok := fileCache[path]
	if ok {
		ff.readersCount++
	}
	cm.cacheLock.Unlock()

	return ff, ok
}

func (cm *inMemoryCacheManager) SetFileToCache(cacheKind CacheKind, path string, ff *fsFile) *fsFile {
	fileCache := cm.getFsCache(cacheKind)

	cm.cacheLock.Lock()
	ff1, ok := fileCache[path]
	if !ok {
		fileCache[path] = ff
		ff.readersCount++
	} else {
		ff1.readersCount++
	}
	cm.cacheLock.Unlock()

	if ok {
		// The file has been already opened by another
		// goroutine, so close the current file and use
		// the file opened by another goroutine instead.
		ff.Release()
		ff = ff1
	}

	return ff
}

func (cm *inMemoryCacheManager) handleCleanCache(cleanStop chan struct{}) {
	var pendingFiles []*fsFile

	clean := func() {
		pendingFiles = cm.cleanCache(pendingFiles)
	}

	if cleanStop != nil {
		t := time.NewTicker(cm.cacheDuration / 2)
		for {
			select {
			case <-t.C:
				clean()
			case _, stillOpen := <-cleanStop:
				// Ignore values send on the channel, only stop when it is closed.
				if !stillOpen {
					t.Stop()
					return
				}
			}
		}
	}
	for {
		time.Sleep(cm.cacheDuration / 2)
		clean()
	}
}

func (cm *inMemoryCacheManager) cleanCache(pendingFiles []*fsFile) []*fsFile {
	var filesToRelease []*fsFile

	cm.cacheLock.Lock()

	// Close files which couldn't be closed before due to non-zero
	// readers count on the previous run.
	var remainingFiles []*fsFile
	for _, ff := range pendingFiles {
		if ff.readersCount > 0 {
			remainingFiles = append(remainingFiles, ff)
		} else {
			filesToRelease = append(filesToRelease, ff)
		}
	}
	pendingFiles = remainingFiles

	pendingFiles, filesToRelease = cleanCacheNolock(cm.cache, pendingFiles, filesToRelease, cm.cacheDuration)
	pendingFiles, filesToRelease = cleanCacheNolock(cm.cacheBrotli, pendingFiles, filesToRelease, cm.cacheDuration)
	pendingFiles, filesToRelease = cleanCacheNolock(cm.cacheGzip, pendingFiles, filesToRelease, cm.cacheDuration)
	pendingFiles, filesToRelease = cleanCacheNolock(cm.cacheZstd, pendingFiles, filesToRelease, cm.cacheDuration)

	cm.cacheLock.Unlock()

	for _, ff := range filesToRelease {
		ff.Release()
	}

	return pendingFiles
}

func cleanCacheNolock(
	cache map[string]*fsFile, pendingFiles, filesToRelease []*fsFile, cacheDuration time.Duration,
) ([]*fsFile, []*fsFile) {
	t := time.Now()
	for k, ff := range cache {
		if t.Sub(ff.t) > cacheDuration {
			if ff.readersCount > 0 {
				// There are pending readers on stale file handle,
				// so we cannot close it. Put it into pendingFiles
				// so it will be closed later.
				pendingFiles = append(pendingFiles, ff)
			} else {
				filesToRelease = append(filesToRelease, ff)
			}
			delete(cache, k)
		}
	}
	return pendingFiles, filesToRelease
}

func (h *fsHandler) pathToFilePath(path string) string {
	if _, ok := h.filesystem.(*osFS); !ok {
		if len(path) < 1 {
			return path
		}
		return path[1:]
	}
	return filepath.FromSlash(h.root + path)
}

func (h *fsHandler) filePathToCompressed(filePath string) string {
	if h.root == h.compressRoot {
		return filePath
	}
	if !strings.HasPrefix(filePath, h.root) {
		return filePath
	}
	return filepath.FromSlash(h.compressRoot + filePath[len(h.root):])
}

func (h *fsHandler) handleRequest(ctx *RequestCtx) {
	var path []byte
	if h.pathRewrite != nil {
		path = h.pathRewrite(ctx)
	} else {
		path = ctx.Path()
	}
	hasTrailingSlash := len(path) > 0 && path[len(path)-1] == '/'

	if n := bytes.IndexByte(path, 0); n >= 0 {
		ctx.Logger().Printf("cannot serve path with nil byte at position %d: %q", n, path)
		ctx.Error("Are you a hacker?", StatusBadRequest)
		return
	}
	if h.pathRewrite != nil {
		// There is no need to check for '/../' if path = ctx.Path(),
		// since ctx.Path must normalize and sanitize the path.

		if n := bytes.Index(path, strSlashDotDotSlash); n >= 0 {
			ctx.Logger().Printf("cannot serve path with '/../' at position %d due to security reasons: %q", n, path)
			ctx.Error("Internal Server Error", StatusInternalServerError)
			return
		}
	}

	mustCompress := false
	fileCacheKind := defaultCacheKind
	fileEncoding := ""
	byteRange := ctx.Request.Header.peek(strRange)
	if len(byteRange) == 0 && h.compress {
		switch {
		case h.compressBrotli && ctx.Request.Header.HasAcceptEncodingBytes(strBr):
			mustCompress = true
			fileCacheKind = brotliCacheKind
			fileEncoding = "br"
		case h.compressZstd && ctx.Request.Header.HasAcceptEncodingBytes(strZstd):
			mustCompress = true
			fileCacheKind = zstdCacheKind
			fileEncoding = "zstd"
		case ctx.Request.Header.HasAcceptEncodingBytes(strGzip):
			mustCompress = true
			fileCacheKind = gzipCacheKind
			fileEncoding = "gzip"
		}
	}

	originalPathStr := string(path)
	pathStr := originalPathStr
	if hasTrailingSlash {
		pathStr = originalPathStr[:len(originalPathStr)-1]
	}

	ff, ok := h.cacheManager.GetFileFromCache(fileCacheKind, originalPathStr)
	if !ok {
		filePath := h.pathToFilePath(pathStr)

		var err error
		ff, err = h.openFSFile(filePath, mustCompress, fileEncoding)
		if mustCompress && err == errNoCreatePermission {
			ctx.Logger().Printf("insufficient permissions for saving compressed file for %q. Serving uncompressed file. "+
				"Allow write access to the directory with this file in order to improve fasthttp performance", filePath)
			mustCompress = false
			ff, err = h.openFSFile(filePath, mustCompress, fileEncoding)
		}

		if errors.Is(err, errDirIndexRequired) {
			if !hasTrailingSlash {
				ctx.RedirectBytes(append(path, '/'), StatusFound)
				return
			}
			ff, err = h.openIndexFile(ctx, filePath, mustCompress, fileEncoding)
			if err != nil {
				ctx.Logger().Printf("cannot open dir index %q: %v", filePath, err)
				ctx.Error("Directory index is forbidden", StatusForbidden)
				return
			}
		} else if err != nil {
			ctx.Logger().Printf("cannot open file %q: %v", filePath, err)
			if h.pathNotFound == nil {
				ctx.Error("Cannot open requested path", StatusNotFound)
			} else {
				ctx.SetStatusCode(StatusNotFound)
				h.pathNotFound(ctx)
			}
			return
		}

		ff = h.cacheManager.SetFileToCache(fileCacheKind, originalPathStr, ff)
	}

	if !ctx.IfModifiedSince(ff.lastModified) {
		ff.decReadersCount()
		ctx.NotModified()
		return
	}

	r, err := ff.NewReader()
	if err != nil {
		ctx.Logger().Printf("cannot obtain file reader for path=%q: %v", path, err)
		ctx.Error("Internal Server Error", StatusInternalServerError)
		return
	}

	hdr := &ctx.Response.Header
	if ff.compressed {
		switch fileEncoding {
		case "br":
			hdr.SetContentEncodingBytes(strBr)
			hdr.addVaryBytes(strAcceptEncoding)
		case "gzip":
			hdr.SetContentEncodingBytes(strGzip)
			hdr.addVaryBytes(strAcceptEncoding)
		case "zstd":
			hdr.SetContentEncodingBytes(strZstd)
			hdr.addVaryBytes(strAcceptEncoding)
		}
	}

	statusCode := StatusOK
	contentLength := ff.contentLength
	if h.acceptByteRange {
		hdr.setNonSpecial(strAcceptRanges, strBytes)
		if len(byteRange) > 0 {
			startPos, endPos, err := ParseByteRange(byteRange, contentLength)
			if err != nil {
				_ = r.(io.Closer).Close()
				ctx.Logger().Printf("cannot parse byte range %q for path=%q: %v", byteRange, path, err)
				ctx.Error("Range Not Satisfiable", StatusRequestedRangeNotSatisfiable)
				return
			}

			if err = r.(byteRangeUpdater).UpdateByteRange(startPos, endPos); err != nil {
				_ = r.(io.Closer).Close()
				ctx.Logger().Printf("cannot seek byte range %q for path=%q: %v", byteRange, path, err)
				ctx.Error("Internal Server Error", StatusInternalServerError)
				return
			}

			hdr.SetContentRange(startPos, endPos, contentLength)
			contentLength = endPos - startPos + 1
			statusCode = StatusPartialContent
		}
	}

	hdr.setNonSpecial(strLastModified, ff.lastModifiedStr)
	if !ctx.IsHead() {
		ctx.SetBodyStream(r, contentLength)
	} else {
		ctx.Response.ResetBody()
		ctx.Response.SkipBody = true
		ctx.Response.Header.SetContentLength(contentLength)
		if rc, ok := r.(io.Closer); ok {
			if err := rc.Close(); err != nil {
				ctx.Logger().Printf("cannot close file reader: %v", err)
				ctx.Error("Internal Server Error", StatusInternalServerError)
				return
			}
		}
	}
	hdr.noDefaultContentType = true
	if len(hdr.ContentType()) == 0 {
		ctx.SetContentType(ff.contentType)
	}
	ctx.SetStatusCode(statusCode)
}

type byteRangeUpdater interface {
	UpdateByteRange(startPos, endPos int) error
}

// ParseByteRange parses 'Range: bytes=...' header value.
//
// It follows https://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.35 .
func ParseByteRange(byteRange []byte, contentLength int) (startPos, endPos int, err error) {
	b := byteRange
	if !bytes.HasPrefix(b, strBytes) {
		return 0, 0, fmt.Errorf("unsupported range units: %q. Expecting %q", byteRange, strBytes)
	}

	b = b[len(strBytes):]
	if len(b) == 0 || b[0] != '=' {
		return 0, 0, fmt.Errorf("missing byte range in %q", byteRange)
	}
	b = b[1:]

	n := bytes.IndexByte(b, '-')
	if n < 0 {
		return 0, 0, fmt.Errorf("missing the end position of byte range in %q", byteRange)
	}

	if n == 0 {
		v, err := ParseUint(b[n+1:])
		if err != nil {
			return 0, 0, err
		}
		startPos := contentLength - v
		if startPos < 0 {
			startPos = 0
		}
		return startPos, contentLength - 1, nil
	}

	if startPos, err = ParseUint(b[:n]); err != nil {
		return 0, 0, err
	}
	if startPos >= contentLength {
		return 0, 0, fmt.Errorf("the start position of byte range cannot exceed %d. byte range %q", contentLength-1, byteRange)
	}

	b = b[n+1:]
	if len(b) == 0 {
		return startPos, contentLength - 1, nil
	}

	if endPos, err = ParseUint(b); err != nil {
		return 0, 0, err
	}
	if endPos >= contentLength {
		endPos = contentLength - 1
	}
	if endPos < startPos {
		return 0, 0, fmt.Errorf("the start position of byte range cannot exceed the end position. byte range %q", byteRange)
	}
	return startPos, endPos, nil
}

func (h *fsHandler) openIndexFile(ctx *RequestCtx, dirPath string, mustCompress bool, fileEncoding string) (*fsFile, error) {
	for _, indexName := range h.indexNames {
		indexFilePath := indexName
		if dirPath != "" {
			indexFilePath = dirPath + "/" + indexName
		}

		ff, err := h.openFSFile(indexFilePath, mustCompress, fileEncoding)
		if err == nil {
			return ff, nil
		}
		if mustCompress && err == errNoCreatePermission {
			ctx.Logger().Printf("insufficient permissions for saving compressed file for %q. Serving uncompressed file. "+
				"Allow write access to the directory with this file in order to improve fasthttp performance", indexFilePath)
			mustCompress = false
			return h.openFSFile(indexFilePath, mustCompress, fileEncoding)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("cannot open file %q: %w", indexFilePath, err)
		}
	}

	if !h.generateIndexPages {
		return nil, fmt.Errorf("cannot access directory without index page. Directory %q", dirPath)
	}

	return h.createDirIndex(ctx, dirPath, mustCompress, fileEncoding)
}

var (
	errDirIndexRequired   = errors.New("directory index required")
	errNoCreatePermission = errors.New("no 'create file' permissions")
)

func (h *fsHandler) createDirIndex(ctx *RequestCtx, dirPath string, mustCompress bool, fileEncoding string) (*fsFile, error) {
	w := &bytebufferpool.ByteBuffer{}

	base := ctx.URI()

	// io/fs doesn't support ReadDir with empty path.
	if dirPath == "" {
		dirPath = "."
	}

	basePathEscaped := html.EscapeString(string(base.Path()))
	_, _ = fmt.Fprintf(w, "<html><head><title>%s</title><style>.dir { font-weight: bold }</style></head><body>", basePathEscaped)
	_, _ = fmt.Fprintf(w, "<h1>%s</h1>", basePathEscaped)
	_, _ = fmt.Fprintf(w, "<ul>")

	if len(basePathEscaped) > 1 {
		var parentURI URI
		base.CopyTo(&parentURI)
		parentURI.Update(string(base.Path()) + "/..")
		parentPathEscaped := html.EscapeString(string(parentURI.Path()))
		_, _ = fmt.Fprintf(w, `<li><a href="%s" class="dir">..</a></li>`, parentPathEscaped)
	}

	dirEntries, err := fs.ReadDir(h.filesystem, dirPath)
	if err != nil {
		return nil, err
	}

	fm := make(map[string]fs.FileInfo, len(dirEntries))
	filenames := make([]string, 0, len(dirEntries))
nestedContinue:
	for _, de := range dirEntries {
		name := de.Name()
		for _, cfs := range h.compressedFileSuffixes {
			if strings.HasSuffix(name, cfs) {
				// Do not show compressed files on index page.
				continue nestedContinue
			}
		}
		fi, err := de.Info()
		if err != nil {
			ctx.Logger().Printf("cannot fetch information from dir entry %q: %v, skip", name, err)

			continue nestedContinue
		}

		fm[name] = fi
		filenames = append(filenames, name)
	}

	var u URI
	base.CopyTo(&u)
	u.Update(string(u.Path()) + "/")

	sort.Strings(filenames)
	for _, name := range filenames {
		u.Update(name)
		pathEscaped := html.EscapeString(string(u.Path()))
		fi := fm[name]
		auxStr := "dir"
		className := "dir"
		if !fi.IsDir() {
			auxStr = fmt.Sprintf("file, %d bytes", fi.Size())
			className = "file"
		}
		_, _ = fmt.Fprintf(w, `<li><a href="%s" class="%s">%s</a>, %s, last modified %s</li>`,
			pathEscaped, className, html.EscapeString(name), auxStr, fsModTime(fi.ModTime()))
	}

	_, _ = fmt.Fprintf(w, "</ul></body></html>")

	if mustCompress {
		var zbuf bytebufferpool.ByteBuffer
		switch fileEncoding {
		case "br":
			zbuf.B = AppendBrotliBytesLevel(zbuf.B, w.B, CompressDefaultCompression)
		case "gzip":
			zbuf.B = AppendGzipBytesLevel(zbuf.B, w.B, CompressDefaultCompression)
		case "zstd":
			zbuf.B = AppendZstdBytesLevel(zbuf.B, w.B, CompressZstdDefault)
		}
		w = &zbuf
	}

	dirIndex := w.B
	lastModified := time.Now()
	ff := &fsFile{
		h:               h,
		dirIndex:        dirIndex,
		contentType:     "text/html; charset=utf-8",
		contentLength:   len(dirIndex),
		compressed:      mustCompress,
		lastModified:    lastModified,
		lastModifiedStr: AppendHTTPDate(nil, lastModified),

		t: lastModified,
	}
	return ff, nil
}

const (
	fsMinCompressRatio        = 0.8
	fsMaxCompressibleFileSize = 8 * 1024 * 1024
)

func (h *fsHandler) compressAndOpenFSFile(filePath, fileEncoding string) (*fsFile, error) {
	f, err := h.filesystem.Open(filePath)
	if err != nil {
		return nil, err
	}

	fileInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("cannot obtain info for file %q: %w", filePath, err)
	}

	if fileInfo.IsDir() {
		_ = f.Close()
		return nil, errDirIndexRequired
	}

	if strings.HasSuffix(filePath, h.compressedFileSuffixes[fileEncoding]) ||
		fileInfo.Size() > fsMaxCompressibleFileSize ||
		!isFileCompressible(f, fsMinCompressRatio) {
		return h.newFSFile(f, fileInfo, false, filePath, "")
	}

	compressedFilePath := h.filePathToCompressed(filePath)

	if _, ok := h.filesystem.(*osFS); !ok {
		return h.newCompressedFSFileCache(f, fileInfo, compressedFilePath, fileEncoding)
	}

	if compressedFilePath != filePath {
		if err := os.MkdirAll(filepath.Dir(compressedFilePath), 0o750); err != nil {
			return nil, err
		}
	}
	compressedFilePath += h.compressedFileSuffixes[fileEncoding]

	absPath, err := filepath.Abs(compressedFilePath)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("cannot determine absolute path for %q: %v", compressedFilePath, err)
	}

	flock := getFileLock(absPath)
	flock.Lock()
	ff, err := h.compressFileNolock(f, fileInfo, filePath, compressedFilePath, fileEncoding)
	flock.Unlock()

	return ff, err
}

func (h *fsHandler) compressFileNolock(
	f fs.File, fileInfo fs.FileInfo, filePath, compressedFilePath, fileEncoding string,
) (*fsFile, error) {
	// Attempt to open compressed file created by another concurrent
	// goroutine.
	// It is safe opening such a file, since the file creation
	// is guarded by file mutex - see getFileLock call.
	if _, err := os.Stat(compressedFilePath); err == nil {
		_ = f.Close()
		return h.newCompressedFSFile(compressedFilePath, fileEncoding)
	}

	// Create temporary file, so concurrent goroutines don't use
	// it until it is created.
	tmpFilePath := compressedFilePath + ".tmp"
	zf, err := os.Create(tmpFilePath)
	if err != nil {
		_ = f.Close()
		if !errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("cannot create temporary file %q: %w", tmpFilePath, err)
		}
		return nil, errNoCreatePermission
	}
	switch fileEncoding {
	case "br":
		zw := acquireStacklessBrotliWriter(zf, CompressDefaultCompression)
		_, err = copyZeroAlloc(zw, f)
		if errf := zw.Flush(); err == nil {
			err = errf
		}
		releaseStacklessBrotliWriter(zw, CompressDefaultCompression)
	case "gzip":
		zw := acquireStacklessGzipWriter(zf, CompressDefaultCompression)
		_, err = copyZeroAlloc(zw, f)
		if errf := zw.Flush(); err == nil {
			err = errf
		}
		releaseStacklessGzipWriter(zw, CompressDefaultCompression)
	case "zstd":
		zw := acquireStacklessZstdWriter(zf, CompressZstdDefault)
		_, err = copyZeroAlloc(zw, f)
		if errf := zw.Flush(); err == nil {
			err = errf
		}
		releaseStacklessZstdWriter(zw, CompressZstdDefault)
	}
	_ = zf.Close()
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("error when compressing file %q to %q: %w", filePath, tmpFilePath, err)
	}
	if err = os.Chtimes(tmpFilePath, time.Now(), fileInfo.ModTime()); err != nil {
		return nil, fmt.Errorf("cannot change modification time to %v for tmp file %q: %v",
			fileInfo.ModTime(), tmpFilePath, err)
	}
	if err = os.Rename(tmpFilePath, compressedFilePath); err != nil {
		return nil, fmt.Errorf("cannot move compressed file from %q to %q: %w", tmpFilePath, compressedFilePath, err)
	}
	return h.newCompressedFSFile(compressedFilePath, fileEncoding)
}

// newCompressedFSFileCache use memory cache compressed files.
func (h *fsHandler) newCompressedFSFileCache(f fs.File, fileInfo fs.FileInfo, filePath, fileEncoding string) (*fsFile, error) {
	var (
		w   = &bytebufferpool.ByteBuffer{}
		err error
	)

	switch fileEncoding {
	case "br":
		zw := acquireStacklessBrotliWriter(w, CompressDefaultCompression)
		_, err = copyZeroAlloc(zw, f)
		if errf := zw.Flush(); err == nil {
			err = errf
		}
		releaseStacklessBrotliWriter(zw, CompressDefaultCompression)
	case "gzip":
		zw := acquireStacklessGzipWriter(w, CompressDefaultCompression)
		_, err = copyZeroAlloc(zw, f)
		if errf := zw.Flush(); err == nil {
			err = errf
		}
		releaseStacklessGzipWriter(zw, CompressDefaultCompression)
	case "zstd":
		zw := acquireStacklessZstdWriter(w, CompressZstdDefault)
		_, err = copyZeroAlloc(zw, f)
		if errf := zw.Flush(); err == nil {
			err = errf
		}
		releaseStacklessZstdWriter(zw, CompressZstdDefault)
	}
	defer func() { _ = f.Close() }()

	if err != nil {
		return nil, fmt.Errorf("error when compressing file %q: %w", filePath, err)
	}

	seeker, ok := f.(io.Seeker)
	if !ok {
		return nil, errors.New("not implemented io.Seeker")
	}
	if _, err = seeker.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	ext := fileExtension(fileInfo.Name(), false, h.compressedFileSuffixes[fileEncoding])
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		data, err := readFileHeader(f, false, fileEncoding)
		if err != nil {
			return nil, fmt.Errorf("cannot read header of the file %q: %w", fileInfo.Name(), err)
		}
		contentType = http.DetectContentType(data)
	}

	dirIndex := w.B
	lastModified := fileInfo.ModTime()
	ff := &fsFile{
		h:               h,
		dirIndex:        dirIndex,
		contentType:     contentType,
		contentLength:   len(dirIndex),
		compressed:      true,
		lastModified:    lastModified,
		lastModifiedStr: AppendHTTPDate(nil, lastModified),

		t: time.Now(),
	}

	return ff, nil
}

func (h *fsHandler) newCompressedFSFile(filePath, fileEncoding string) (*fsFile, error) {
	f, err := h.filesystem.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open compressed file %q: %w", filePath, err)
	}
	fileInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("cannot obtain info for compressed file %q: %w", filePath, err)
	}
	return h.newFSFile(f, fileInfo, true, filePath, fileEncoding)
}

func (h *fsHandler) openFSFile(filePath string, mustCompress bool, fileEncoding string) (*fsFile, error) {
	filePathOriginal := filePath
	if mustCompress {
		filePath += h.compressedFileSuffixes[fileEncoding]
	}
	f, err := h.filesystem.Open(filePath)
	if err != nil {
		if mustCompress && errors.Is(err, fs.ErrNotExist) {
			return h.compressAndOpenFSFile(filePathOriginal, fileEncoding)
		}

		// If the file is not found and the path is empty, let's return errDirIndexRequired error.
		if filePath == "" && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid)) {
			return nil, errDirIndexRequired
		}

		return nil, err
	}

	fileInfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("cannot obtain info for file %q: %w", filePath, err)
	}

	if fileInfo.IsDir() {
		_ = f.Close()
		if mustCompress {
			return nil, fmt.Errorf("directory with unexpected suffix found: %q. Suffix: %q",
				filePath, h.compressedFileSuffixes[fileEncoding])
		}
		return nil, errDirIndexRequired
	}

	if mustCompress {
		fileInfoOriginal, err := fs.Stat(h.filesystem, filePathOriginal)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("cannot obtain info for original file %q: %w", filePathOriginal, err)
		}

		// Only re-create the compressed file if there was more than a second between the mod times.
		// On macOS the gzip seems to truncate the nanoseconds in the mod time causing the original file
		// to look newer than the gzipped file.
		if fileInfoOriginal.ModTime().Sub(fileInfo.ModTime()) >= time.Second {
			// The compressed file became stale. Re-create it.
			_ = f.Close()
			_ = os.Remove(filePath)
			return h.compressAndOpenFSFile(filePathOriginal, fileEncoding)
		}
	}

	return h.newFSFile(f, fileInfo, mustCompress, filePath, fileEncoding)
}

func (h *fsHandler) newFSFile(f fs.File, fileInfo fs.FileInfo, compressed bool, filePath, fileEncoding string) (*fsFile, error) {
	n := fileInfo.Size()
	contentLength := int(n)
	if n != int64(contentLength) {
		_ = f.Close()
		return nil, fmt.Errorf("too big file: %d bytes", n)
	}

	// detect content-type
	ext := fileExtension(fileInfo.Name(), compressed, h.compressedFileSuffixes[fileEncoding])
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		data, err := readFileHeader(f, compressed, fileEncoding)
		if err != nil {
			return nil, fmt.Errorf("cannot read header of the file %q: %w", fileInfo.Name(), err)
		}
		contentType = http.DetectContentType(data)
	}

	lastModified := fileInfo.ModTime()
	ff := &fsFile{
		h:               h,
		f:               f,
		filename:        filePath,
		contentType:     contentType,
		contentLength:   contentLength,
		compressed:      compressed,
		lastModified:    lastModified,
		lastModifiedStr: AppendHTTPDate(nil, lastModified),

		t: time.Now(),
	}
	return ff, nil
}

func readFileHeader(f io.Reader, compressed bool, fileEncoding string) ([]byte, error) {
	r := f
	var (
		br  *brotli.Reader
		zr  *gzip.Reader
		zsr *zstd.Decoder
	)
	if compressed {
		var err error
		switch fileEncoding {
		case "br":
			if br, err = acquireBrotliReader(f); err != nil {
				return nil, err
			}
			r = br
		case "gzip":
			if zr, err = acquireGzipReader(f); err != nil {
				return nil, err
			}
			r = zr
		case "zstd":
			if zsr, err = acquireZstdReader(f); err != nil {
				return nil, err
			}
			r = zsr
		}
	}

	lr := &io.LimitedReader{
		R: r,
		N: 512,
	}
	data, err := io.ReadAll(lr)
	seeker, ok := f.(io.Seeker)
	if !ok {
		return nil, errors.New("must implement io.Seeker")
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	if br != nil {
		releaseBrotliReader(br)
	}

	if zr != nil {
		releaseGzipReader(zr)
	}

	if zsr != nil {
		releaseZstdReader(zsr)
	}

	return data, err
}

func stripLeadingSlashes(path []byte, stripSlashes int) []byte {
	for stripSlashes > 0 && len(path) > 0 {
		if path[0] != '/' {
			// developer sanity-check
			panic("BUG: path must start with slash")
		}
		n := bytes.IndexByte(path[1:], '/')
		if n < 0 {
			path = path[:0]
			break
		}
		path = path[n+1:]
		stripSlashes--
	}
	return path
}

func stripTrailingSlashes(path []byte) []byte {
	for len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	return path
}

func fileExtension(path string, compressed bool, compressedFileSuffix string) string {
	if compressed && strings.HasSuffix(path, compressedFileSuffix) {
		path = path[:len(path)-len(compressedFileSuffix)]
	}
	n := strings.LastIndexByte(path, '.')
	if n < 0 {
		return ""
	}
	return path[n:]
}

// FileLastModified returns last modified time for the file.
func FileLastModified(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return zeroTime, err
	}
	fileInfo, err := f.Stat()
	_ = f.Close()
	if err != nil {
		return zeroTime, err
	}
	return fsModTime(fileInfo.ModTime()), nil
}

func fsModTime(t time.Time) time.Time {
	return t.In(time.UTC).Truncate(time.Second)
}

var filesLockMap sync.Map

func getFileLock(absPath string) *sync.Mutex {
	v, _ := filesLockMap.LoadOrStore(absPath, &sync.Mutex{})
	filelock := v.(*sync.Mutex)
	return filelock
}

var _ fs.FS = (*osFS)(nil)

type osFS struct{}

func (o *osFS) Open(name string) (fs.File, error)     { return os.Open(name) }
func (o *osFS) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
