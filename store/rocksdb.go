package store

import (
	"github.com/tecbot/gorocksdb"
)

var _ KV = (*rocksdbKV)(nil)

type rocksdbKV struct {
	dir string

	opts *gorocksdb.Options
	bbto *gorocksdb.BlockBasedTableOptions
	ro   *gorocksdb.ReadOptions
	wo   *gorocksdb.WriteOptions

	db    *gorocksdb.DB
	cache *gorocksdb.Cache
}

func (r *rocksdbKV) Close() error {
	if r.db != nil {
		r.db.Close()
	}

	if r.cache != nil {
		r.cache.Destroy()
	}

	if r.bbto != nil {
		r.bbto.Destroy()
	}

	if r.opts != nil {
		r.opts.Destroy()
	}

	if r.wo != nil {
		r.wo.Destroy()
	}

	if r.ro != nil {
		r.ro.Destroy()
	}

	r.db = nil
	return nil
}

func (r *rocksdbKV) Get(key []byte) ([]byte, error) {
	return r.db.GetBytes(r.ro, key)
}

func (r *rocksdbKV) MultiGet(keys ...[]byte) (bufs [][]byte, err error) {
	slices, err := r.db.MultiGet(r.ro, keys...)
	if err != nil {
		return nil, err
	}

	for _, slice := range slices {
		bufs = append(bufs, slice.Data())
	}

	return
}

func (r *rocksdbKV) Put(key, value []byte) error {
	return r.db.Put(r.wo, key, value)
}

func (r *rocksdbKV) NewWriteBatch() WriteBatch {
	return gorocksdb.NewWriteBatch()
}

func (r *rocksdbKV) CommitWriteBatch(batch WriteBatch) error {
	wb, ok := batch.(*gorocksdb.WriteBatch)
	if !ok {
		panic("provided batch is not type *gorocksdb.WriteBatch")
	}

	return r.db.Write(r.wo, wb)
}

func NewRocksDB(dir string) (*rocksdbKV, error) {
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockSize(32 * 1024)

	cache := gorocksdb.NewLRUCache(1024 * 512)
	bbto.SetBlockCache(cache)

	opts := gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)

	opts.SetCreateIfMissing(true)
	opts.SetUseFsync(true)

	opts.SetUseDirectIOForFlushAndCompaction(true)

	opts.SetCompression(gorocksdb.NoCompression)

	opts.SetWriteBufferSize(256 * 1024 * 1024)

	opts.SetLevel0FileNumCompactionTrigger(8)
	opts.SetLevel0SlowdownWritesTrigger(17)
	opts.SetLevel0StopWritesTrigger(24)

	opts.SetMaxWriteBufferNumber(25)

	opts.SetMaxBytesForLevelBase(4 * 1024 * 1024 * 1024)
	opts.SetMaxBytesForLevelMultiplier(2)

	opts.SetTargetFileSizeBase(256 * 1024 * 1024)
	opts.SetTargetFileSizeMultiplier(1)

	db, err := gorocksdb.OpenDb(opts, dir)
	if err != nil {
		return nil, err
	}

	wo := gorocksdb.NewDefaultWriteOptions()
	ro := gorocksdb.NewDefaultReadOptions()

	kv := &rocksdbKV{
		dir: dir,

		opts: opts,
		bbto: bbto,

		db:    db,
		cache: cache,

		wo: wo,
		ro: ro,
	}

	return kv, nil
}
