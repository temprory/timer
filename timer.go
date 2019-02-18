package timer

import (
	"container/heap"
	"math"
	"sync"
	"time"
)

const (
	TIME_FOREVER = time.Duration(math.MaxInt64)
)

var (
	DefaultTimer = New("default")
)

type TimerItem struct {
	Index    int
	Expire   time.Time
	Callback func()
}

type Timers []*TimerItem

func (tm Timers) Len() int { return len(tm) }

func (tm Timers) Less(i, j int) bool {
	return tm[i].Expire.Before(tm[j].Expire)
}

func (tm Timers) Swap(i, j int) {
	tm[i], tm[j] = tm[j], tm[i]
	tm[i].Index, tm[j].Index = i, j
}

func (tm *Timers) Push(x interface{}) {
	n := len(*tm)
	item := x.(*TimerItem)
	item.Index = n
	*tm = append(*tm, item)
}

func (tm *Timers) Remove(i int) {
	n := tm.Len() - 1
	if n != i {
		(*tm).Swap(i, n)
		(*tm)[n] = nil
		*tm = (*tm)[:n]
		heap.Fix(tm, i)
	} else {
		(*tm)[n] = nil
		*tm = (*tm)[:n]
	}

}

func (tm *Timers) Pop() interface{} {
	old := *tm
	n := len(old)
	if n > 0 {
		tm.Swap(0, n-1)
		item := old[n-1]
		item.Index = -1
		*tm = old[:n-1]
		heap.Fix(tm, 0)
		return item
	} else {
		return nil
	}
}

func (tm *Timers) Head() *TimerItem {
	t := *tm
	n := t.Len()
	if n > 0 {
		return t[0]
	}
	return nil
}

type Timer struct {
	sync.Mutex
	tag     string
	timers  Timers
	trigger *time.Timer
	chStop  chan struct{}
}

func (tm *Timer) After(timeout time.Duration) <-chan struct{} {
	tm.Lock()
	defer tm.Unlock()

	ch := make(chan struct{}, 1)
	item := &TimerItem{
		Index:  len(tm.timers),
		Expire: time.Now().Add(timeout),
		Callback: func() {
			ch <- struct{}{}
		},
	}
	tm.timers = append(tm.timers, item)
	heap.Fix(&(tm.timers), item.Index)
	if head := tm.timers.Head(); head == item {
		tm.trigger.Reset(head.Expire.Sub(time.Now()))
	}

	return ch
}

func (tm *Timer) AfterFunc(timeout time.Duration, cb func()) *TimerItem {
	return tm.Once(timeout, cb)
}

func (tm *Timer) Once(timeout time.Duration, cb func()) *TimerItem {
	tm.Lock()
	defer tm.Unlock()

	item := &TimerItem{
		Index:    len(tm.timers),
		Expire:   time.Now().Add(timeout),
		Callback: cb,
	}
	tm.timers = append(tm.timers, item)
	heap.Fix(&(tm.timers), item.Index)
	if head := tm.timers.Head(); head == item {
		tm.trigger.Reset(head.Expire.Sub(time.Now()))
	}

	return item
}

func (tm *Timer) Schedule(delay time.Duration, interval time.Duration, cb func()) *TimerItem {
	tm.Lock()
	defer tm.Unlock()

	var (
		item *TimerItem
		now  = time.Now()
	)

	item = &TimerItem{
		Index:  len(tm.timers),
		Expire: now.Add(delay),
		Callback: func() {
			now = time.Now()
			tm.Lock()
			item.Index = len(tm.timers)
			item.Expire = now.Add(interval)
			tm.timers = append(tm.timers, item)
			heap.Fix(&(tm.timers), item.Index)

			tm.Unlock()

			cb()

			if head := tm.timers.Head(); head == item {
				tm.trigger.Reset(head.Expire.Sub(now))
			}
		},
	}

	tm.timers = append(tm.timers, item)
	heap.Fix(&(tm.timers), item.Index)
	if head := tm.timers.Head(); head == item {
		tm.trigger.Reset(head.Expire.Sub(now))
	}

	return item
}

func (tm *Timer) Cancel(item *TimerItem) {
	tm.Lock()
	defer tm.Unlock()
	n := tm.timers.Len()
	if n == 0 {
		logDebug("Timer(%v) Cancel Error: Timer Size Is 0!", tm.tag)
		return
	}
	if item.Index > 0 && item.Index < n {
		if item != tm.timers[item.Index] {
			logDebug("Timer(%v) Cancel Error: Invalid Item!", tm.tag)
			return
		}
		tm.timers.Remove(item.Index)
	} else if item.Index == 0 {
		if item != tm.timers[item.Index] {
			logDebug("Timer(%v) Cancel Error: Invalid Item!", tm.tag)
			return
		}
		tm.timers.Remove(item.Index)
		if head := tm.timers.Head(); head != nil && head != item {
			tm.trigger.Reset(head.Expire.Sub(time.Now()))
		}
	} else {
		logDebug("Timer(%v) Cancel Error: Invalid Index: %d", tm.tag, item.Index)
	}
}

func (tm *Timer) Reset(item *TimerItem, delay time.Duration) {
	tm.Lock()
	defer tm.Unlock()

	n := tm.timers.Len()
	if n == 0 {
		logDebug("Timer(%S) Reset Error: Timer Size Is 0!", tm.tag)
		return
	}
	if item.Index < n {
		if item != tm.timers[item.Index] {
			logDebug("Timer(%S) Reset Error: Invalid Item!", tm.tag)
			return
		}
		item.Expire = time.Now().Add(delay)
		heap.Fix(&(tm.timers), item.Index)
	} else {
		logDebug("Timer(%S) Reset Error: Invalid Item!", tm.tag)
	}
}

func (tm *Timer) Size() int {
	tm.Lock()
	defer tm.Unlock()
	return len(tm.timers)
}

func (tm *Timer) Stop() {
	tm.Lock()
	defer tm.Unlock()
	close(tm.chStop)
	tm.trigger.Stop()
}

func (tm *Timer) once() {
	defer handlePanic()
	tm.Lock()
	item := tm.timers.Pop()
	if item != nil {
		if head := tm.timers.Head(); head != nil {
			tm.trigger.Reset(head.Expire.Sub(time.Now()))
		}
	} else {
		tm.trigger.Reset(TIME_FOREVER)
	}
	tm.Unlock()

	if item != nil {
		item.(*TimerItem).Callback()
	}
}

func After(timeout time.Duration) <-chan struct{} {
	return DefaultTimer.After(timeout)
}

func AfterFunc(timeout time.Duration, cb func()) *TimerItem {
	return DefaultTimer.AfterFunc(timeout, cb)
}

func Once(timeout time.Duration, cb func()) *TimerItem {
	return DefaultTimer.Once(timeout, cb)
}

func Schedule(delay time.Duration, interval time.Duration, cb func()) *TimerItem {
	return DefaultTimer.Schedule(delay, interval, cb)
}

func Cancel(item *TimerItem) {
	DefaultTimer.Cancel(item)
}

func New(tag string) *Timer {
	tm := &Timer{
		tag:     tag,
		trigger: time.NewTimer(TIME_FOREVER),
		chStop:  make(chan struct{}),
		timers:  []*TimerItem{},
	}

	go func() {
		defer logDebug("timer(%v) stopped", tag)
		for {
			select {
			case <-tm.trigger.C:
				tm.once()
			case <-tm.chStop:
				return
			}
		}
	}()

	return tm
}
