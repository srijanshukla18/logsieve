package ingestion

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "sync"
    "time"

    "github.com/rs/zerolog"

    "github.com/logsieve/logsieve/pkg/config"
    "github.com/logsieve/logsieve/pkg/metrics"
)

// DiskBuffer writes flushed batches to disk and reads them back in order.
// It aims for pragmatic durability without heavy dependencies.
type DiskBuffer struct {
    cfg      config.IngestionConfig
    logger   zerolog.Logger
    metrics  *metrics.Registry
    name     string

    in       chan *LogEntry
    batches  chan []*LogEntry

    mu       sync.RWMutex
    closed   bool
    wg       sync.WaitGroup
    dir      string
}

func NewDiskBuffer(cfg config.IngestionConfig, logger zerolog.Logger) *DiskBuffer {
    db := &DiskBuffer{
        cfg:     cfg,
        logger:  logger.With().Str("component", "disk_buffer").Logger(),
        in:      make(chan *LogEntry, cfg.BufferSize),
        batches: make(chan []*LogEntry, 100),
        dir:     cfg.DiskPath,
    }
    _ = os.MkdirAll(db.dir, 0o755)
    db.wg.Add(2)
    go db.writer()
    go db.reader()
    return db
}

func (d *DiskBuffer) WithMetrics(m *metrics.Registry, name string) *DiskBuffer {
    d.metrics = m
    d.name = name
    return d
}

func (d *DiskBuffer) Add(entry *LogEntry) error {
    d.mu.RLock()
    if d.closed {
        d.mu.RUnlock()
        return fmt.Errorf("buffer closed")
    }
    d.mu.RUnlock()
    select {
    case d.in <- entry:
        return nil
    default:
        if d.metrics != nil {
            d.metrics.QueueDroppedEntriesTotal.WithLabelValues(d.name).Inc()
        }
        return fmt.Errorf("disk buffer input full")
    }
}

func (d *DiskBuffer) GetBatch() <-chan []*LogEntry { return d.batches }

func (d *DiskBuffer) Close() error {
    d.mu.Lock()
    if d.closed { d.mu.Unlock(); return nil }
    d.closed = true
    close(d.in)
    d.mu.Unlock()
    d.wg.Wait()
    close(d.batches)
    return nil
}

func (d *DiskBuffer) writer() {
    defer d.wg.Done()
    batch := make([]*LogEntry, 0, d.cfg.MaxBatchSize)
    ticker := time.NewTicker(d.cfg.FlushInterval)
    defer ticker.Stop()
    flush := func() {
        if len(batch) == 0 { return }
        // Serialize batch to a file
        ts := time.Now().UnixNano()
        fn := filepath.Join(d.dir, fmt.Sprintf("ready-%d.jsonl", ts))
        f, err := os.Create(fn)
        if err != nil {
            d.logger.Error().Err(err).Str("file", fn).Msg("create spool file failed")
            batch = batch[:0]
            return
        }
        w := bufio.NewWriter(f)
        for _, e := range batch {
            b, _ := json.Marshal(e)
            w.Write(b)
            w.WriteByte('\n')
        }
        w.Flush()
        f.Close()
        batch = batch[:0]
        d.evictIfNeeded()
    }
    for {
        select {
        case e, ok := <-d.in:
            if !ok { flush(); return }
            batch = append(batch, e)
            if len(batch) >= d.cfg.MaxBatchSize { flush() }
        case <-ticker.C:
            flush()
        }
    }
}

func (d *DiskBuffer) reader() {
    defer d.wg.Done()
    for {
        if d.isClosed() && !d.hasReadyFiles() {
            return
        }
        files := d.listReady()
        if len(files) == 0 {
            time.Sleep(100 * time.Millisecond)
            continue
        }
        // Process oldest first
        fn := files[0]
        if err := d.sendFile(fn); err != nil {
            d.logger.Error().Err(err).Str("file", fn).Msg("failed to process spool file")
            // On error, drop the file to avoid blocking forever
            _ = os.Remove(fn)
        }
    }
}

func (d *DiskBuffer) sendFile(path string) error {
    f, err := os.Open(path)
    if err != nil { return err }
    defer f.Close()
    scanner := bufio.NewScanner(f)
    batch := make([]*LogEntry, 0, d.cfg.MaxBatchSize)
    flush := func() {
        if len(batch) == 0 { return }
        select {
        case d.batches <- batch:
        default:
            if d.metrics != nil {
                d.metrics.QueueDroppedBatchesTotal.WithLabelValues(d.name).Inc()
                d.metrics.QueueDroppedEntriesTotal.WithLabelValues(d.name).Add(float64(len(batch)))
            }
        }
        batch = make([]*LogEntry, 0, d.cfg.MaxBatchSize)
    }
    for scanner.Scan() {
        var e LogEntry
        if err := json.Unmarshal(scanner.Bytes(), &e); err != nil { continue }
        batch = append(batch, &e)
        if len(batch) >= d.cfg.MaxBatchSize { flush() }
    }
    if err := scanner.Err(); err != nil { return err }
    flush()
    // Remove processed file
    return os.Remove(path)
}

func (d *DiskBuffer) listReady() []string {
    entries, err := os.ReadDir(d.dir)
    if err != nil { return nil }
    var files []string
    for _, e := range entries {
        if e.IsDir() { continue }
        name := e.Name()
        if strings.HasPrefix(name, "ready-") && strings.HasSuffix(name, ".jsonl") {
            files = append(files, filepath.Join(d.dir, name))
        }
    }
    sort.Strings(files)
    return files
}

func (d *DiskBuffer) hasReadyFiles() bool { return len(d.listReady()) > 0 }

func (d *DiskBuffer) isClosed() bool { d.mu.RLock(); defer d.mu.RUnlock(); return d.closed }

func (d *DiskBuffer) evictIfNeeded() {
    // Evict oldest files when exceeding size budget
    var total int64
    files := d.listReady()
    for _, f := range files {
        if fi, err := os.Stat(f); err == nil { total += fi.Size() }
    }
    for total > d.cfg.MaxDiskBytes && len(files) > 0 {
        oldest := files[0]
        if fi, err := os.Stat(oldest); err == nil { total -= fi.Size() }
        _ = os.Remove(oldest)
        files = files[1:]
        if d.metrics != nil {
            // Count eviction as dropped batch; entries unknown so add 0 to entries
            d.metrics.QueueDroppedBatchesTotal.WithLabelValues(d.name).Inc()
        }
    }
}

func (d *DiskBuffer) Stats() BufferStats {
    // Provide approximate stats
    entries := d.listReady()
    return BufferStats{BufferSize: len(entries), BufferCap: int(d.cfg.MaxDiskBytes), BatchQueueSize: len(d.batches), BatchQueueCap: cap(d.batches)}
}

