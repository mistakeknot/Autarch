package review

type Queue struct {
    ids []string
}

func NewQueue() *Queue { return &Queue{ids: []string{}} }

func (q *Queue) Add(id string) { q.ids = append(q.ids, id) }

func (q *Queue) Len() int { return len(q.ids) }
