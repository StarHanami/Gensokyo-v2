package idmap

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/hoshinonyaruko/gensokyo/config"
	"github.com/hoshinonyaruko/gensokyo/mylog"
	"go.etcd.io/bbolt"
)

// 新 idmap 系统：三库分离
//   idmap-identity.db — GroupOpenID ↔ 虚拟群ID + UserOpenID ↔ 虚拟用户ID（永久数据）
//   idmap-msg.db      — 真实 message_id ↔ 虚拟 message_id（临时缓存）
//   旧 idmap.db       — 仅读取（惰性迁移），不再写入

const (
	IdentityDBName = "idmap-identity.db"
	MsgDBName      = "idmap-msg.db"

	IdentityBucketName = "ids"
	MsgBucketName      = "cache"

	IdentityCounterKey = "currentRow"
	MsgCounterKey      = "currentRow"
)

var (
	identityDB *bbolt.DB // 身份映射 DB（group + user 共用）
	msgDB      *bbolt.DB // 消息 ID 缓存 DB
	newDBOnce  sync.Once
)

// initNewDBs 初始化新 DB（惰性，首次调用时打开）
func initNewDBs() {
	newDBOnce.Do(func() {
		var err error

		identityDB, err = bbolt.Open(IdentityDBName, 0600, nil)
		if err != nil {
			mylog.Fatalf("Error opening %s: %v", IdentityDBName, err)
		}

		msgDB, err = bbolt.Open(MsgDBName, 0600, nil)
		if err != nil {
			mylog.Fatalf("Error opening %s: %v", MsgDBName, err)
		}

		// 创建 buckets
		for _, d := range []struct {
			db     *bbolt.DB
			name   string
			bucket string
		}{
			{identityDB, IdentityDBName, IdentityBucketName},
			{msgDB, MsgDBName, MsgBucketName},
		} {
			err = d.db.Update(func(tx *bbolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists([]byte(d.bucket))
				return err
			})
			if err != nil {
				mylog.Fatalf("Error creating bucket in %s: %v", d.name, err)
			}
		}

		mylog.Printf("新 idmap 数据库已就绪: %s, %s", IdentityDBName, MsgDBName)

		// 检测旧 DB，启动后台惰性迁移
		if hasOldDB() {
			mylog.Printf("检测到旧 idmap.db，启动后台静默迁移")
			go backgroundMigration()
		}
	})
}

// hasOldDB 检查旧 idmap.db 是否存在
func hasOldDB() bool {
	// 如果旧 db 已经打开（由原有初始化逻辑负责），则返回 true
	return db != nil
}

// ---------------------------------------------------------------------------
// 身份映射（Group + User）
// ---------------------------------------------------------------------------

// storeIdentity 写入身份映射（内部核心函数）
// openID: 真实 OpenID（32位 hex 字符串）
// 返回: 虚拟数字 ID
func storeIdentity(openID string) (int64, error) {
	initNewDBs()

	var newRow int64
	key := uinKey(openID)
	revPrefix := uinRowKey("")

	err := identityDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(IdentityBucketName))

		// 已存在直接返回
		existing := b.Get([]byte(key))
		if existing != nil {
			newRow = int64(binary.BigEndian.Uint64(existing))
			return nil
		}

		// 分配虚拟 ID
		if !config.GetHashIDValue() {
			currentRowBytes := b.Get([]byte(IdentityCounterKey))
			if currentRowBytes == nil {
				newRow = 1
			} else {
				newRow = int64(binary.BigEndian.Uint64(currentRowBytes)) + 1
			}
		} else {
			var err error
			maxDigits := 18
			for digits := 9; digits <= maxDigits; digits++ {
				newRow, err = GenerateRowID(openID, digits)
				if err != nil {
					return err
				}
				rowKey := fmt.Sprintf("row-%d", newRow)
				if b.Get([]byte(rowKey)) == nil {
					break
				}
				if digits == maxDigits {
					return fmt.Errorf("unable to find unique row ID after %d attempts", maxDigits-8)
				}
			}
		}

		rowBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(rowBytes, uint64(newRow))

		if !config.GetHashIDValue() {
			b.Put([]byte(IdentityCounterKey), rowBytes)
		}
		b.Put([]byte(key), rowBytes)
		b.Put([]byte(revPrefix+strconv.FormatInt(newRow, 10)), []byte(key))

		if config.GetIdmapIsolation() && config.GetIdmapLegacyCompat() {
			b.Put([]byte(openID), rowBytes)
		}
		return nil
	})

	// 写旧库保持双写兼容（惰性迁移期）
	if err == nil {
		dualWriteToOldDB(key, openID, newRow)
	}

	return newRow, err
}

// retrieveIdentity 根据虚拟 ID 查找真实 OpenID（惰性：新库找不到时查旧库）
func retrieveIdentity(virtualID string) (string, error) {
	initNewDBs()

	var id string
	revKey := uinRowKey(virtualID)

	err := identityDB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(IdentityBucketName))
		idBytes := b.Get([]byte(revKey))
		if idBytes == nil {
			return ErrKeyNotFound
		}
		id = stripUinPrefix(string(idBytes))
		return nil
	})
	if err == nil && id != "" {
		return id, nil
	}

	// 惰性迁移：新库查不到，查旧库
	id, err = lazyMigrateIdentity(virtualID)
	if err == nil {
		return id, nil
	}

	return "", ErrKeyNotFound
}

// lazyMigrateIdentity 从旧 idmap.db 读取并写入新库
func lazyMigrateIdentity(virtualID string) (string, error) {
	if !hasOldDB() {
		return "", ErrKeyNotFound
	}

	id, err := RetrieveRowByID(virtualID)
	if err != nil {
		return "", err
	}

	// 写入新库，下次就不用查旧库了
	rawKey := stripUinPrefix(id)
	if len(rawKey) == 32 {
		// 32位 OpenID，写入新库
		writeBackIdentity(virtualID, id)
	}

	return id, nil
}

// writeBackIdentity 将旧库数据回写到新库
func writeBackIdentity(virtualID string, openID string) {
	key := uinKey(openID)
	revPrefix := uinRowKey("")

	_ = identityDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(IdentityBucketName))

		rowBytes := make([]byte, 8)
		vID, _ := strconv.ParseInt(virtualID, 10, 64)
		binary.BigEndian.PutUint64(rowBytes, uint64(vID))

		b.Put([]byte(key), rowBytes)
		b.Put([]byte(revPrefix+virtualID), []byte(key))
		return nil
	})
}

// dualWriteToOldDB 双写到旧库（兼容期）
func dualWriteToOldDB(key, openID string, rowID int64) {
	if !hasOldDB() {
		return
	}

	revKey := "row-" + strconv.FormatInt(rowID, 10)
	rowBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(rowBytes, uint64(rowID))

	_ = db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketName))
		// 仅在旧库没有该条目时才写入
		if b.Get([]byte(key)) == nil {
			b.Put([]byte(key), rowBytes)
			b.Put([]byte(revKey), []byte(key))
			if config.GetIdmapIsolation() && config.GetIdmapLegacyCompat() {
				b.Put([]byte(openID), rowBytes)
			}
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// 后台静默迁移
// ---------------------------------------------------------------------------

var migrationStarted bool

// backgroundMigration 后台扫描旧库，将数据分批搬入新库
func backgroundMigration() {
	if migrationStarted {
		return
	}
	migrationStarted = true

	go func() {
		// 先迁移 identity（ids 桶）
		migrateBucket(BucketName, IdentityBucketName, identityDB, "identity")
		// 再迁移消息缓存（cache 桶）
		migrateBucket(CacheBucketName, MsgBucketName, msgDB, "msg")

		mylog.Printf("[idmap] 后台迁移完成，旧 idmap.db 可安全删除")
	}()
}

// migrateBucket 将旧库一个桶中的数据逐条搬入新库
func migrateBucket(oldBucket, newBucket string, newDB *bbolt.DB, label string) {
	if !hasOldDB() {
		return
	}

	type entry struct {
		key   string
		value string
	}

	batchSize := 100
	total := 0

	for {
		// 从旧库读一批
		batch, done, err := readOldDBBatch(oldBucket, batchSize)
		if err != nil || len(batch) == 0 {
			if done {
				mylog.Printf("[idmap] 迁移 %s 完成，共 %d 条", label, total)
			}
			return
		}

		// 写入新库（跳过已存在的）
		written := writeBatchToNewDB(newDB, newBucket, batch)
		total += written

		if done {
			mylog.Printf("[idmap] 迁移 %s 完成，共 %d 条", label, total)
			return
		}

		// 每批之间稍作暂停，避免 CPU 争抢
		if total%(batchSize*10) == 0 {
			mylog.Printf("[idmap] 迁移 %s 进度: %d 条", label, total)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// readOldDBBatch 从旧库中读取一批尚未迁移的条目
var oldCursorKey []byte // 批次游标

func readOldDBBatch(bucketName string, limit int) ([]entry, bool, error) {
	var entries []entry
	done := false

	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			done = true
			return nil
		}

		c := b.Cursor()
		k, v := c.Seek(oldCursorKey)
		if k == nil {
			done = true
			return nil
		}

		// 跳过计数器 key
		for i := 0; i < limit && k != nil; i++ {
			keyStr := string(k)
			if keyStr == IdentityCounterKey || keyStr == MsgCounterKey {
				k, v = c.Next()
				continue
			}
			entries = append(entries, entry{key: keyStr, value: string(v)})
			k, v = c.Next()
		}

		if k == nil {
			done = true
		} else {
			oldCursorKey = k
		}
		return nil
	})

	if err != nil {
		return nil, true, err
	}
	if len(entries) == 0 {
		done = true
	}
	return entries, done, nil
}

// writeBatchToNewDB 将一批条目写入新库（跳过已存在的）
func writeBatchToNewDB(newDB *bbolt.DB, bucketName string, entries []entry) int {
	written := 0
	_ = newDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		for _, e := range entries {
			if b.Get([]byte(e.key)) == nil {
				b.Put([]byte(e.key), []byte(e.value))
				written++
			}
		}
		return nil
	})
	return written
}

// ResetMigrationCursor 重置迁移游标（重新扫描全部）
func ResetMigrationCursor() {
	oldCursorKey = nil
	migrationStarted = false
}

// StoreGroupID 存储群 OpenID → 虚拟群 ID
func StoreGroupID(groupOpenID string) (int64, error) {
	return storeIdentity(groupOpenID)
}

// StoreUserID 存储用户 OpenID → 虚拟用户 ID
func StoreUserID(userOpenID string) (int64, error) {
	return storeIdentity(userOpenID)
}

// RetrieveGroupID 虚拟群 ID → 真实群 OpenID
func RetrieveGroupID(virtualID string) (string, error) {
	return retrieveIdentity(virtualID)
}

// RetrieveUserID 虚拟用户 ID → 真实用户 OpenID
func RetrieveUserID(virtualID string) (string, error) {
	return retrieveIdentity(virtualID)
}

// StoreMsgID 存储真实消息 ID → 虚拟消息 ID
func StoreMsgID(realMsgID string) (int64, error) {
	initNewDBs()

	var newRow int64
	key := uinKey(realMsgID)
	revPrefix := uinRowKey("")

	err := msgDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(MsgBucketName))

		existing := b.Get([]byte(key))
		if existing != nil {
			newRow = int64(binary.BigEndian.Uint64(existing))
			return nil
		}

		currentRowBytes := b.Get([]byte(MsgCounterKey))
		if currentRowBytes == nil {
			newRow = 1
		} else {
			newRow = int64(binary.BigEndian.Uint64(currentRowBytes)) + 1
		}

		rowBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(rowBytes, uint64(newRow))
		b.Put([]byte(MsgCounterKey), rowBytes)
		b.Put([]byte(key), rowBytes)
		b.Put([]byte(revPrefix+strconv.FormatInt(newRow, 10)), []byte(key))

		// 惰性迁移：同时写旧 cache 桶
		if hasOldDB() {
			_ = db.Update(func(tx2 *bbolt.Tx) error {
				b2 := tx2.Bucket([]byte(CacheBucketName))
				if b2.Get([]byte(key)) == nil {
					b2.Put([]byte(key), rowBytes)
					b2.Put([]byte(revPrefix+strconv.FormatInt(newRow, 10)), []byte(key))
				}
				return nil
			})
		}
		return nil
	})

	return newRow, err
}

// RetrieveMsgID 虚拟消息 ID → 真实消息 ID
func RetrieveMsgID(virtualID string) (string, error) {
	initNewDBs()

	var id string
	revKey := uinRowKey(virtualID)

	err := msgDB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(MsgBucketName))
		idBytes := b.Get([]byte(revKey))
		if idBytes == nil {
			return ErrKeyNotFound
		}
		id = stripUinPrefix(string(idBytes))
		return nil
	})
	if err == nil && id != "" {
		return id, nil
	}

	// 惰性迁移
	if hasOldDB() {
		id, err = RetrieveRowByCache(virtualID)
		if err == nil && id != "" {
			// 写回新库
			_ = msgDB.Update(func(tx *bbolt.Tx) error {
				b := tx.Bucket([]byte(MsgBucketName))
				key := uinKey(id)
				rowBytes := make([]byte, 8)
				vID, _ := strconv.ParseInt(virtualID, 10, 64)
				binary.BigEndian.PutUint64(rowBytes, uint64(vID))
				b.Put([]byte(key), rowBytes)
				b.Put([]byte(revKey), []byte(key))
				return nil
			})
			return id, nil
		}
	}

	return "", ErrKeyNotFound
}

// CleanMsgDB 清理消息 ID 缓存 DB（可安全删除）
func CleanMsgDB() error {
	initNewDBs()
	return msgDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(MsgBucketName))
		return b.ForEach(func(k, v []byte) error {
			return b.Delete(k)
		})
	})
}

// newDBStore 由旧 StoreIDv2 调用，双写到新 identity DB
func newDBStore(openID string, virtualID int64) {
	initNewDBs()
	key := uinKey(openID)
	revPrefix := uinRowKey("")

	_ = identityDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(IdentityBucketName))
		rowBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(rowBytes, uint64(virtualID))
		if b.Get([]byte(key)) == nil {
			b.Put([]byte(key), rowBytes)
			b.Put([]byte(revPrefix+strconv.FormatInt(virtualID, 10)), []byte(key))
		}
		return nil
	})
}

// newDBLookup 由旧 RetrieveRowByIDv2 调用，优先查新 identity DB
func newDBLookup(virtualID string) (string, bool) {
	initNewDBs()
	revKey := uinRowKey(virtualID)
	var result string

	err := identityDB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(IdentityBucketName))
		v := b.Get([]byte(revKey))
		if v == nil {
			return ErrKeyNotFound
		}
		result = stripUinPrefix(string(v))
		return nil
	})

	if err == nil && result != "" {
		return result, true
	}
	return "", false
}

// newDBMsgStore 由旧 StoreCachev2 调用，双写到新 msg DB
func newDBMsgStore(realMsgID string, virtualID int64) {
	initNewDBs()
	key := uinKey(realMsgID)
	revPrefix := uinRowKey("")

	_ = msgDB.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(MsgBucketName))
		rowBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(rowBytes, uint64(virtualID))
		if b.Get([]byte(key)) == nil {
			b.Put([]byte(key), rowBytes)
			b.Put([]byte(revPrefix+strconv.FormatInt(virtualID, 10)), []byte(key))
		}
		return nil
	})
}

// newDBMsgLookup 由旧 RetrieveRowByCachev2 调用，优先查新 msg DB
func newDBMsgLookup(virtualID string) (string, bool) {
	initNewDBs()
	revKey := uinRowKey(virtualID)
	var result string

	err := msgDB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(MsgBucketName))
		v := b.Get([]byte(revKey))
		if v == nil {
			return ErrKeyNotFound
		}
		result = stripUinPrefix(string(v))
		return nil
	})

	if err == nil && result != "" {
		return result, true
	}
	return "", false
}
