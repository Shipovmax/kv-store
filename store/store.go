// Package store реализует потокобезопасное in-memory key-value хранилище
// с поддержкой TTL и фоновой очисткой истёкших записей.
package store

import (
	"context"
	"sync"
	"time"
)

// defaultSweepInterval — период фоновой проверки истёкших ключей,
// используемый по умолчанию, если StartSweep вызван без явного интервала.
const defaultSweepInterval = time.Second

// entry — хранимое значение вместе с моментом истечения TTL.
// Нулевое значение expiresAt означает отсутствие TTL (ключ бессрочный).
type entry struct {
	value     string
	expiresAt time.Time
}

// expired сообщает, истёк ли TTL записи на момент now.
func (e entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

// KVStore — потокобезопасное in-memory хранилище пар ключ-значение.
// Конкурентный доступ защищён sync.RWMutex: чтение (Get) допускает
// произвольное число параллельных читателей, запись (Set/Delete)
// эксклюзивна. Для read-heavy нагрузки (типичной для KV-кэша) это
// снижает contention по сравнению с sync.Mutex.
//
// Нулевое значение KVStore непригодно для использования — создавать
// экземпляр следует через New.
type KVStore struct {
	mu   sync.RWMutex
	data map[string]entry
}

// New создаёт пустое хранилище, готовое к использованию.
func New() *KVStore {
	return &KVStore{
		data: make(map[string]entry),
	}
}

// Set записывает значение по ключу. Если ttl > 0, ключ истечёт через
// указанный промежуток времени и станет недоступен для Get; при ttl <= 0
// ключ хранится бессрочно (до явного Delete или перезаписи).
func (s *KVStore) Set(key, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := entry{value: value}
	if ttl > 0 {
		e.expiresAt = time.Now().Add(ttl)
	}
	s.data[key] = e
}

// Get возвращает значение по ключу. Второе возвращаемое значение — false,
// если ключ отсутствует или его TTL истёк; в последнем случае запись
// физически удаляется из-под фоновой очистки не мгновенно, а на ближайшем
// проходе sweep-горутины, но для читателя истёкший ключ уже недоступен.
func (s *KVStore) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.data[key]
	if !ok || e.expired(time.Now()) {
		return "", false
	}
	return e.value, true
}

// Delete удаляет ключ. Удаление отсутствующего ключа — no-op.
func (s *KVStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// StartSweep запускает фоновую горутину, периодически (с периодом interval,
// либо defaultSweepInterval при interval <= 0) удаляющую истёкшие ключи из
// хранилища. Альтернатива — time.AfterFunc на каждый ключ — при большом
// числе записей порождает горутину на ключ; единый тикер даёт O(1) по числу
// фоновых горутин ценой задержки удаления до одного периода sweep.
//
// Горутина завершается при отмене ctx — это единственный путь её
// завершения, вызывающий код обязан в конце концов отменить ctx, иначе
// горутина будет работать до конца жизни процесса.
func (s *KVStore) StartSweep(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultSweepInterval
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.sweepExpired()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// sweepExpired удаляет из хранилища все записи с истёкшим TTL.
// Сложность: O(n) по числу ключей за проход, O(1) дополнительной памяти —
// иного пути обойти все записи без вспомогательного индекса по expiresAt нет;
// при очень большом числе ключей с TTL альтернативой было бы min-heap по
// expiresAt для амортизированного удаления без полного скана, но это
// оправдано только при высокой доле TTL-ключей и жёстких требованиях к
// задержке освобождения памяти.
func (s *KVStore) sweepExpired() {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.data {
		if v.expired(now) {
			delete(s.data, k)
		}
	}
}
