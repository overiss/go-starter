# starter

`starter` - Go-библиотека для запуска воркеров сервиса с graceful shutdown.

## Зачем это нужно

В сервисе обычно есть несколько фоновых компонентов:

- HTTP-сервер;
- consumer очереди;
- планировщик задач;
- другие долгоживущие воркеры.

У всех одинаковый жизненный цикл: запустить вместе, дождаться сигнала остановки и корректно завершить.  
`starter` берет эту координацию на себя.

## Основная идея

Каждый компонент реализует интерфейс `Worker`:

```go
type Worker interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
```

- `Start(ctx)` запускает работу воркера и обычно блокируется до остановки/ошибки.
- `Stop(ctx)` останавливает воркер корректно.

Инициализация и запуск выглядят просто:

```go
s := starter.Init(worker1, worker2, worker3)
err := s.Run(ctx)
```

Если нужно запускать один и тот же воркер в несколько потоков:

```go
s := starter.InitWithConfig(
	starter.WorkerConfig{Worker: queueWorker, Threads: 4},
	starter.WorkerConfig{Worker: httpWorker}, // Threads не указан -> 1
)
err := s.Run(ctx)
```

## Установка

```bash
go get github.com/overiss/starter
```

## Быстрый старт

```go
package main

import (
	"context"
	"log"

	starter "github.com/overiss/starter"
)

type ExampleWorker struct{}

func (w *ExampleWorker) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (w *ExampleWorker) Stop(ctx context.Context) error {
	return nil
}

func main() {
	s := starter.Init(&ExampleWorker{})
	if err := s.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

## Как работает `Run`

`Run(ctx)`:

- запускает каждый воркер в отдельной горутине;
- слушает системные сигналы `SIGINT` и `SIGTERM`;
- завершает работу, если:
  - пришел сигнал ОС;
  - отменен входной `ctx`;
  - один из `Start` вернул ошибку;
- после этого вызывает `Stop` у всех воркеров параллельно;
- при `InitWithConfig` вызывает `Start` каждого воркера `Threads` раз;
- если `Threads` не задан или <= 0, используется `1`;
- на остановку дается внутренний таймаут `15s`.

## Почему библиотека оптимизирована

Внутри `starter` сделан упор на предсказуемую работу под нагрузкой:

- количество стартовых горутин считается заранее при инициализации, без повторного пересчета в `Run`;
- буфер канала ошибок старта подбирается под общее число потоков, чтобы избежать лишних блокировок при массовых падениях;
- подготовка структуры раннера выполняется один раз в `Init`/`InitWithConfig`, чтобы уменьшить лишние аллокации в горячем пути запуска;
- остановка воркеров идет параллельно и агрегирует ошибки без промежуточных тяжелых структур;
- сигналы ОС обрабатываются единым раннером, что убирает дублирование логики shutdown в каждом сервисе.

## Публичный API

- `Init(workers ...Worker) *Runner` - создает раннер из списка воркеров.
- `InitWithConfig(configs ...WorkerConfig) *Runner` - создает раннер из конфигов вида `{Worker, Threads}`.
- `(*Runner).Run(ctx context.Context) error` - запускает воркеры и блокируется до завершения.
- `NamedWorker` (опционально) - позволяет задать имя воркера для более понятных ошибок.

## Рекомендации для реализации воркеров

- В `Start` обязательно обрабатывай `ctx.Done()`.
- `Stop` лучше делать идемпотентным (повторный вызов не ломает состояние).
- `Stop` должен уважать контекст, чтобы завершиться в пределах timeout.

```go
type NamedWorker interface {
	Name() string
}
```
