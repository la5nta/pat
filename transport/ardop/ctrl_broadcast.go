// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package ardop

import (
	"log"
	"time"
)

type receiver interface {
	sendChan() chan<- ctrlMsg
	doneChan() <-chan struct{}
}

type StateReceiver struct {
	cs   <-chan State
	msgs chan ctrlMsg
	done chan struct{}
}

func (r StateReceiver) States() <-chan State {
	return r.cs
}

func (r StateReceiver) Close() {
	close(r.done)
}

func (r StateReceiver) sendChan() chan<- ctrlMsg {
	return r.msgs
}

func (r StateReceiver) doneChan() <-chan struct{} {
	return r.done
}

type rawReceiver struct {
	msgs chan ctrlMsg  // read from this to receive broadcasts
	done chan struct{} // close this to unregister
}

func (r rawReceiver) Msgs() <-chan ctrlMsg {
	return r.msgs
}

func (r rawReceiver) Close() {
	close(r.done)
}

func (r rawReceiver) sendChan() chan<- ctrlMsg {
	return r.msgs
}

func (r rawReceiver) doneChan() <-chan struct{} {
	return r.done
}

type broadcaster struct {
	msgs     chan ctrlMsg  // send on this will broadcast
	register chan receiver // send on this will register
}

func newBroadcaster() broadcaster {
	receivers := make([]receiver, 0, 1)

	b := broadcaster{
		msgs:     make(chan ctrlMsg),
		register: make(chan receiver),
	}

	go func() {
		defer func() {
			for _, r := range receivers {
				close(r.sendChan())
			}
			receivers = nil
		}()

		for {
			select {
			case r := <-b.register:
				receivers = append(receivers, r)
			case msg, ok := <-b.msgs:
				if !ok {
					return
				}
				for i := 0; i < len(receivers); i++ {
					r := receivers[i]
					select {
					case <-r.doneChan():
						// the receiver is done, remove it
						close(r.sendChan())
						receivers = append(receivers[:i], receivers[i+1:]...)
						i-- //REVIEW this
					case r.sendChan() <- msg:
						// Message sent
					case <-time.After(500 * time.Millisecond): // This is a hack - some of the clients don't close properly
						if debugEnabled() {
							log.Println("Receiver timeout!")
						}
						close(r.sendChan())
						receivers = append(receivers[:i], receivers[i+1:]...)
						i-- //REVIEW this
					}
				}
			}
		}
	}()

	return b
}

func (b *broadcaster) Listen() rawReceiver {
	r := rawReceiver{
		make(chan ctrlMsg, 3),
		make(chan struct{}),
	}
	b.register <- r
	return r
}

func (b *broadcaster) ListenState() StateReceiver {
	cs := make(chan State)
	r := StateReceiver{
		msgs: make(chan ctrlMsg),
		done: make(chan struct{}),
		cs:   cs,
	}
	go func() {
		for msg := range r.msgs {
			if msg.cmd == cmdNewState {
				cs <- msg.State()
			}
		}
		close(cs)
	}()
	b.register <- r
	return r
}

func (b *broadcaster) Send(msg ctrlMsg) {
	b.msgs <- msg
}

func (b *broadcaster) Close() {
	close(b.msgs)
}
