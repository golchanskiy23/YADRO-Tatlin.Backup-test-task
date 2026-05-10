# DNS-manager

Клиент-серверное приложение на Go для управления DNS-серверами на удалённом хосте через `/etc/resolv.conf`.

Сервер запускается на целевой машине и предоставляет gRPC API. Клиент — CLI-инструмент для взаимодействия с сервером.

## Требования

- Go 1.21+
- `protoc` + плагины `protoc-gen-go` и `protoc-gen-go-grpc` — только если нужно пересобрать proto

## Быстрый старт

```bash
# сборка
make build

# или вручную
go build ./...
```

### Запуск сервера

```bash
# на удалённом хосте, по умолчанию порт 50051
go run ./cmd/server

# с кастомным портом
go run ./cmd/server --port 9090

# через переменные окружения
DNS_MANAGER_PORT=9090 
DNS_MANAGER_LOG_LEVEL=DEBUG 
go run ./cmd/server
```

Флаги сервера:

| Флаг          | Env                     | По умолчанию | Описание                                         |
|---------------|-------------------------|--------------|--------------------------------------------------|
| `--port`      | `DNS_MANAGER_PORT`      | `50051`      | TCP-порт                                         |
| `--log-level` | `DNS_MANAGER_LOG_LEVEL` | `INFO`       | Уровень логов | `DEBUG`, `INFO`, `WARN`, `ERROR` |

### Запуск клиента

```bash
# список DNS-серверов
go run ./cmd/client list

# добавить DNS-сервер
go run ./cmd/client add 8.8.8.8

# удалить DNS-сервер
go run ./cmd/client remove 8.8.8.8

# подключиться к нестандартному адресу
go run ./cmd/client --server 192.168.1.10:9090 list

# справка
go run ./cmd/client --help
go run ./cmd/client add --help
```

Флаги клиента:

| Флаг       | По умолчанию      | Описание                            |
|------------|-------------------|-------------------------------------|
| `--server` | `localhost:50051` | Адрес сервера в формате `host:port` |

## Тесты

```bash
make test

# с покрытием
make cover
```

## Пересборка proto (при изменении .proto файла)

```bash
make proto
```

## Структура проекта

```
dns-manager/
├── cmd/
│   ├── server/         # точка входа сервера
│   └── client/         # точка входа CLI-клиента
├── internal/
│   ├── manager/        # бизнес-логика: чтение и запись /etc/resolv.conf
│   └── server/         # реализация gRPC-сервиса
├── proto/dns/          # protobuf-определение API
├── gen/dns/            # сгенерированный Go-код из proto
├── go.mod
├── go.sum
└── Makefile
```

## Архитектура

```
CLI (cobra)  →  gRPC Client  →  [сеть]  →  gRPC Server  →  DNSManagerService  →  Manager  →  /etc/resolv.conf
```

**Manager** (`internal/manager`) — единственный компонент, который читает и пишет `/etc/resolv.conf`. Запись атомарная: через временный файл с последующим `rename(2)`, что исключает повреждение файла при сбое. Конкурентный доступ защищён `sync.RWMutex`.

**DNSManagerService** (`internal/server`) — gRPC-хендлер. Транслирует ошибки Manager в стандартные gRPC-статусы:

| Ошибка            |    gRPC-статус     |
|-------------------|--------------------|
| невалидный IP     | `INVALID_ARGUMENT` |
| IP уже существует | `ALREADY_EXISTS`   |
| IP не найден      | `NOT_FOUND`        |
| ошибка I/O        | `INTERNAL`         |

**Сервер** поддерживает graceful shutdown: при получении `SIGINT`/`SIGTERM` ждёт завершения текущих запросов перед остановкой.

## Диагностика

Сервер ведёт структурированные логи (`log/slog`) для каждого входящего запроса. Для детальной трассировки запустите с уровнем `DEBUG`:

```bash
DNS_MANAGER_LOG_LEVEL=DEBUG 
go run ./cmd/server
```

Каждая запись лога содержит метод, параметры и результат выполнения.

## Замечания

- Авторизация не реализована — предполагается доверенная сеть.
- glibc использует не более 3 записей `nameserver` из `/etc/resolv.conf` — остальные игнорируются на уровне системы.
- На системах с `systemd-resolved` или `NetworkManager` файл `/etc/resolv.conf` может быть симлинком — запись выполняется в реальный файл через `filepath.EvalSymlinks`.
