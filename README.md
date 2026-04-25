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

Если воркер не потокобезопасный, добавь `CloneableWorker` или `ThreadFactory`,
чтобы каждый поток получил отдельный инстанс:

```go
type CloneableWorker interface {
	CloneWorker() starter.Worker
}
```

```go
s := starter.InitWithConfig(
	starter.WorkerConfig{
		Threads: 4,
		ThreadFactory: func(thread int) starter.Worker {
			return NewQueueWorker(thread)
		},
	},
)
```

## Архитектура запуска

`starter` разделяет ответственность:

- ты описываешь только бизнес-логику воркеров (`Start/Stop`);
- раннер управляет параллельным запуском, сигналами ОС, остановкой и агрегацией ошибок.

В результате `main.go` остается компактным, а lifecycle одинаковый во всех сервисах.

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

## Пример: HTTP воркер

```go
type HTTPWorker struct {
	server *http.Server
}

func NewHTTPWorker(addr string, handler http.Handler) *HTTPWorker {
	return &HTTPWorker{
		server: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}
}

func (w *HTTPWorker) Name() string {
	return "http-server"
}

func (w *HTTPWorker) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (w *HTTPWorker) Stop(ctx context.Context) error {
	return w.server.Shutdown(ctx)
}
```

## Пример: очередь с ThreadFactory

Подходит, когда каждый поток должен иметь свой экземпляр consumer:

```go
type QueueWorker struct {
	id int
}

func NewQueueWorker(id int) *QueueWorker {
	return &QueueWorker{id: id}
}

func (w *QueueWorker) Name() string {
	return fmt.Sprintf("queue-%d", w.id)
}

func (w *QueueWorker) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			// read message and handle
		}
	}
}

func (w *QueueWorker) Stop(ctx context.Context) error {
	// close consumer resources
	return nil
}

s := starter.InitWithConfig(
	starter.WorkerConfig{
		Threads: 4,
		ThreadFactory: func(thread int) starter.Worker {
			return NewQueueWorker(thread)
		},
	},
)
```

## Пример: CloneableWorker

Если есть "базовый" воркер и нужно клонировать конфигурацию на потоки:

```go
type MailerWorker struct {
	client *mailer.Client
	queue  string
}

func (w *MailerWorker) CloneWorker() starter.Worker {
	return &MailerWorker{
		client: w.client,
		queue:  w.queue,
	}
}

func (w *MailerWorker) Start(ctx context.Context) error { /* ... */ return nil }
func (w *MailerWorker) Stop(ctx context.Context) error  { /* ... */ return nil }

s := starter.InitWithConfig(
	starter.WorkerConfig{
		Worker:  &MailerWorker{client: mailerClient, queue: "mail"},
		Threads: 8,
	},
)
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

Для расширенного результата используй `RunContext(ctx)`:

- возвращает `RunResult` с причиной остановки (`signal`, `context_cancel`, `worker_error`, `workers_done`);
- содержит `StartError`, `StopError`, `Signal`;
- дает единый `Err()` для совместимости с обычной обработкой ошибок.

## Пример: обработка RunResult

```go
result := s.RunContext(context.Background())

if result.Err() != nil {
	log.Printf("runner failed: %v", result.Err())
}

switch result.Reason {
case starter.StopReasonSignal:
	log.Printf("stopped by signal: %v", result.Signal)
case starter.StopReasonContextCancel:
	log.Printf("stopped by context cancel")
case starter.StopReasonWorkerError:
	log.Printf("stopped because worker failed")
case starter.StopReasonWorkersDone:
	log.Printf("all workers finished")
}
```

## Хуки жизненного цикла

Можно подключить хуки через `Configure(WithHooks(...))`:

- `OnRunnerStart`
- `OnRunnerStop`
- `OnWorkerStart`
- `OnWorkerStop`
- `OnWorkerError`

Это удобно для логов, метрик и трассировки без дублирования кода в каждом воркере.

## Пример: хуки для логов и метрик

```go
s := starter.InitWithConfig(
	starter.WorkerConfig{Worker: httpWorker},
	starter.WorkerConfig{Worker: queueWorker, Threads: 4},
).Configure(
	starter.WithHooks(starter.Hooks{
		OnRunnerStart: func(ctx context.Context) {
			log.Println("runner started")
		},
		OnRunnerStop: func(ctx context.Context, result starter.RunResult) {
			log.Printf("runner stopped: reason=%s err=%v", result.Reason, result.Err())
		},
		OnWorkerStart: func(name string, thread int) {
			metrics.WorkerStarts.WithLabelValues(name).Inc()
			log.Printf("worker start: name=%s thread=%d", name, thread)
		},
		OnWorkerStop: func(name string, thread int) {
			log.Printf("worker stop: name=%s thread=%d", name, thread)
		},
		OnWorkerError: func(name string, thread int, stage starter.WorkerStage, err error) {
			metrics.WorkerErrors.WithLabelValues(name, string(stage)).Inc()
			log.Printf("worker error: name=%s thread=%d stage=%s err=%v", name, thread, stage, err)
		},
	}),
)
```

## Политика ошибок воркеров

Через `Configure(WithErrorPolicy(...))`:

- `ErrorPolicyFailFast` (по умолчанию): первый `Start`-error завершает раннер;
- `ErrorPolicyCollectAll`: ошибки старта копятся, раннер ждет внешней остановки (сигнал/ctx) или завершения всех горутин.

## Пример: выбор политики ошибок

Fail-fast (для API-сервиса, где важна быстрая деградация):

```go
s := starter.Init(apiWorker, queueWorker).Configure(
	starter.WithErrorPolicy(starter.ErrorPolicyFailFast),
)
```

Collect-all (для batch/background, где допустимо продолжать работу):

```go
s := starter.Init(backgroundWorker1, backgroundWorker2).Configure(
	starter.WithErrorPolicy(starter.ErrorPolicyCollectAll),
)
```

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
- `(*Runner).RunContext(ctx context.Context) RunResult` - запускает воркеры и возвращает структурированный результат.
- `(*Runner).Configure(opts ...RunnerOption) *Runner` - включает хуки и политику ошибок.
- `NamedWorker` (опционально) - позволяет задать имя воркера для более понятных ошибок.
- `CloneableWorker` и `ThreadFactory` - создание независимых инстансов воркера под потоки.

## Рекомендации для реализации воркеров

- В `Start` обязательно обрабатывай `ctx.Done()`.
- `Stop` лучше делать идемпотентным (повторный вызов не ломает состояние).
- `Stop` должен уважать контекст, чтобы завершиться в пределах timeout.
- При `Threads > 1` избегай shared mutable state без синхронизации.
- Если воркер не потокобезопасный, используй `ThreadFactory` или `CloneableWorker`.
- Возвращай из `Start` только "реальные" ошибки, `context.Canceled` обычно не нужно пробрасывать как failure.

## Пример полного main.go

```go
func main() {
	ctx := context.Background()

	httpWorker := NewHTTPWorker(":8080", buildHandler())

	s := starter.InitWithConfig(
		starter.WorkerConfig{Worker: httpWorker},
		starter.WorkerConfig{
			Threads: 6,
			ThreadFactory: func(thread int) starter.Worker {
				return NewQueueWorker(thread)
			},
		},
	).Configure(
		starter.WithErrorPolicy(starter.ErrorPolicyFailFast),
		starter.WithHooks(starter.Hooks{
			OnRunnerStart: func(context.Context) {
				log.Println("service started")
			},
			OnRunnerStop: func(_ context.Context, result starter.RunResult) {
				log.Printf("service stopped: reason=%s err=%v", result.Reason, result.Err())
			},
		}),
	)

	if err := s.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

```go
type NamedWorker interface {
	Name() string
}
```
