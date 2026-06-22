# idmap 数据库

## 架构

三库分离设计，避免单个 DB 膨胀导致全部映射丢失。

| 文件 | 用途 | 特性 |
|------|------|------|
| `idmap-identity.db` | OpenID ↔ 虚拟数字 ID | **永久数据**，不会膨胀 |
| `idmap-msg.db` | 真实 message_id ↔ 虚拟 message_id | **临时缓存**，可安全删除 |
| `idmap.db`（旧） | 旧版单库（只读） | 惰性迁移，数据搬完可删除 |

## 新 API

### 身份映射（identity DB）

```go
idmap.StoreGroupID(groupOpenID string) (int64, error)
idmap.StoreUserID(userOpenID string) (int64, error)
idmap.RetrieveGroupID(virtualID string) (string, error)
idmap.RetrieveUserID(virtualID string) (string, error)
```

### 消息缓存（msg DB）

```go
idmap.StoreMsgID(realMsgID string) (int64, error)
idmap.RetrieveMsgID(virtualID string) (string, error)
idmap.CleanMsgDB() error  // 清空消息缓存
```

### 旧 API 兼容

`StoreIDv2` / `RetrieveRowByIDv2` / `StoreCachev2` / `RetrieveRowByCachev2` 仍然可用，内部自动**双写新库 + 惰性迁移**。

## 惰性迁移

```
启动时检测到旧 idmap.db？
  ├── 写入操作 → 同时写旧库和新库
  ├── 读取操作 → 优先查新库
  │     ├── 新库有 → 直接返回
  │     └── 新库无 → 查旧库 → 写回新库 → 返回
  └── 旧库数据全部搬完后，可直接删除 idmap.db
```

## 故障恢复

| 故障 | 恢复方式 |
|------|---------|
| `idmap-msg.db` 损坏/膨胀 | 直接删除，重启自动重建 |
| `idmap-identity.db` 损坏 | 停止后删除，保留 `idmap.db` 重启，惰性迁移自动恢复 |
