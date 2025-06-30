package questionnaire

import "time"

type Questionnaire struct {
	ID          QuestionnaireID
	Code        string
	Title       string
	Description string
	ImgUrl      string
	Version     uint8
	Status      uint8
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   time.Time
	CreatedBy   uint64
	UpdatedBy   uint64
	DeletedBy   uint64
}

func NewQuestionnaire(id QuestionnaireID, code string, title string, imgUrl string, version uint8, status uint8) *Questionnaire {
	return &Questionnaire{
		ID:      id,
		Code:    code,
		Title:   title,
		ImgUrl:  imgUrl,
		Version: version,
		Status:  status,
	}
}

func (q *Questionnaire) SetID(id QuestionnaireID) {
	q.ID = id
}

func (q *Questionnaire) SetCode(code string) {
	q.Code = code
}

func (q *Questionnaire) SetTitle(title string) {
	q.Title = title
}

func (q *Questionnaire) SetDescription(description string) {
	q.Description = description
}

func (q *Questionnaire) SetImgUrl(imgUrl string) {
	q.ImgUrl = imgUrl
}

func (q *Questionnaire) SetVersion(version uint8) {
	q.Version = version
}

func (q *Questionnaire) SetStatus(status uint8) {
	q.Status = status
}

func (q *Questionnaire) SetCreatedAt(createdAt time.Time) {
	q.CreatedAt = createdAt
}

func (q *Questionnaire) SetUpdatedAt(updatedAt time.Time) {
	q.UpdatedAt = updatedAt
}

func (q *Questionnaire) SetDeletedAt(deletedAt time.Time) {
	q.DeletedAt = deletedAt
}

func (q *Questionnaire) SetCreatedBy(createdBy uint64) {
	q.CreatedBy = createdBy
}

func (q *Questionnaire) SetUpdatedBy(updatedBy uint64) {
	q.UpdatedBy = updatedBy
}

func (q *Questionnaire) SetDeletedBy(deletedBy uint64) {
	q.DeletedBy = deletedBy
}
