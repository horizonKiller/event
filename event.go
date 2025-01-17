package event

import (
	"time"
)

// EventPriority is the priority of the event.
type EventPriority int

const (
	// EvRead is readable event.
	EvRead = 1 << iota
	// EvWrite is writable event.
	EvWrite = 1 << iota
	// EvTimeout is timeout event
	EvTimeout = 1 << iota
	// EvSignal is signal event.
	EvSignal = 1 << iota
	// EvClosed is closed event.
	EvClosed = 1 << iota

	// EvPersist is persistent option. If not set, the event will be deleted after it is triggered.
	EvPersist = 0x10
	// EvET is edge-triggered behavior option.
	EvET = 0x20

	// EvListInserted is the flag to indicate the event is in the event list.
	EvListInserted = 0x01
	// EvListActive is the flag to indicate the event is in the active event list.
	EvListActive = 0x02
	// EvListTimeout is the flag to indicate the event is in the timeout event heap.
	EvListTimeout = 0x04

	// High is the high priority.
	High EventPriority = 0b00
	// Middle is the middle priority.
	Middle EventPriority = 0b01
	// Low is the low priority.
	Low EventPriority = 0b10
)

// Event is the event to watch.
type Event struct {
	// ele is the element in the total event list.
	ele *eventListEle
	// activeEle is the element in the active event list.
	activeEle *eventListEle
	// index is the index in the event heap.
	index int

	// fd is the file descriptor to watch.
	fd int
	// events is the events to watch. It can be EvRead, EvWrite, etc.
	events uint32

	// cb is the callback function when the event is triggered.
	cb func(fd int, events uint32, arg interface{})
	// arg is the argument passed to the callback function.
	arg interface{}

	// res is the result passed to the callback function.
	res uint32
	// flags is the status of the event in the event list. It can be EvListInserted or EvListActive.
	flags int

	// timeout is the timeout in milliseconds.
	timeout time.Duration
	// deadline is the deadline for the event.
	deadline int64

	// priority is the priority of the event.
	priority EventPriority
}

func New(fd int, events uint32, callback func(fd int, events uint32, arg interface{}), arg interface{}) *Event {
	return &Event{
		fd:       fd,
		events:   events,
		cb:       callback,
		arg:      arg,
		priority: Middle,
	}
}

func (ev *Event) SetPriority(priority EventPriority) {
	ev.priority = priority
}

func (ev *Event) Assign(fd int, events uint32, callback func(fd int, events uint32, arg interface{}), arg interface{}, priority EventPriority) {
	ev.fd = fd
	ev.events = events
	ev.cb = callback
	ev.arg = arg
	ev.priority = priority
}

// EventBase is the base of all events.
type EventBase struct {
	// poller is the event poller to watch events.
	poller *epoll
	// evList is the list of all events.
	evList *eventList
	// ActiveEvList is the list of active events.
	activeEvLists []*eventList
	// EventHeap is the min heap of timeout events.
	evHeap *eventHeap
}

func NewBase() (*EventBase, error) {
	poller, err := newEpoll()
	if err != nil {
		return nil, err
	}

	return &EventBase{
		poller:        poller,
		evList:        newEventList(),
		activeEvLists: []*eventList{newEventList(), newEventList(), newEventList()},
		evHeap:        newEventHeap(),
	}, nil
}

func (bs *EventBase) AddEvent(ev *Event, timeout time.Duration) error {
	if timeout > 0 && ev.flags&EvListTimeout == 0 {
		ev.timeout = timeout
		ev.deadline = time.Now().Add(timeout).UnixMilli()
		bs.eventQueueInsert(ev, EvListTimeout)
	}

	if ev.flags&EvListInserted == 0 {
		bs.eventQueueInsert(ev, EvListInserted)
		if ev.events&(EvRead|EvWrite|EvSignal) != 0 {
			return bs.poller.add(ev)
		}
	}

	return nil
}

func (bs *EventBase) DelEvent(ev *Event) error {
	if ev.flags&EvListTimeout != 0 {
		bs.eventQueueRemove(ev, EvListTimeout)
	}

	if ev.flags&EvListActive != 0 {
		bs.eventQueueRemove(ev, EvListActive)
	}

	if ev.flags&EvListInserted != 0 {
		bs.eventQueueRemove(ev, EvListInserted)
		if ev.events&(EvRead|EvWrite|EvSignal) != 0 {
			return bs.poller.del(ev)
		}
	}

	return nil
}

func (bs *EventBase) Dispatch() error {
	for {
		err := bs.poller.polling(bs.onActive, bs.waitTime())
		if err != nil {
			return err
		}

		bs.onTimeout()

		bs.handleActiveEvents()
	}
}

func (bs *EventBase) Exit() error {
	return bs.poller.close()
}

func (bs *EventBase) waitTime() int {
	if !bs.evHeap.empty() {
		now := time.Now().UnixMilli()
		ev := bs.evHeap.peekEvent()
		if ev.deadline > now {
			return int(ev.deadline - now)
		}
		return 0
	}
	return -1
}

func (bs *EventBase) onTimeout() {
	now := time.Now().UnixMilli()
	for !bs.evHeap.empty() {
		ev := bs.evHeap.peekEvent()
		if ev.deadline > now {
			break
		}

		bs.DelEvent(ev)

		bs.onActive(ev, EvTimeout)
	}
}

func (bs *EventBase) onActive(ev *Event, res uint32) {
	if ev.flags&EvListActive != 0 {
		ev.res |= res
		return
	}

	ev.res = res
	bs.eventQueueInsert(ev, EvListActive)
}

func (bs *EventBase) handleActiveEvents() {
	for i := range bs.activeEvLists {
		for e := bs.activeEvLists[i].front(); e != nil; {
			next := e.nextEle()
			ev := e.value
			if ev.events&EvPersist != 0 {
				bs.eventQueueRemove(ev, EvListActive)
			} else {
				bs.DelEvent(ev)
			}
			e = next

			if ev.events&EvTimeout != 0 && ev.events&EvPersist != 0 {
				bs.AddEvent(ev, ev.timeout)
			}

			ev.cb(ev.fd, ev.res, ev.arg)
		}
	}
}

func (bs *EventBase) eventQueueInsert(ev *Event, which int) {
	if ev.flags&which != 0 && ev.flags&EvListActive != 0 {
		return
	}

	ev.flags |= which
	switch which {
	case EvListInserted:
		ev.ele = bs.evList.pushBack(ev)
	case EvListActive:
		ev.activeEle = bs.activeEvLists[ev.priority].pushBack(ev)
	case EvListTimeout:
		ev.index = bs.evHeap.pushEvent(ev)
	}
}

func (bs *EventBase) eventQueueRemove(ev *Event, which int) {
	if ev.flags&which == 0 {
		return
	}

	ev.flags &^= which
	switch which {
	case EvListInserted:
		bs.evList.remove(ev.ele)
		ev.ele = nil
	case EvListActive:
		bs.activeEvLists[ev.priority].remove(ev.activeEle)
		ev.activeEle = nil
	case EvListTimeout:
		bs.evHeap.removeEvent(ev.index)
		ev.index = -1
	}
}
