package seedprofile

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/mozillazg/go-pinyin"
)

type Generator struct {
	phonePrefix string
	emailDomain string
}

type Profile struct {
	Index         int
	RunDate       time.Time
	GuardianName  string
	GuardianPhone string
	GuardianEmail string
	ChildName     string
	ChildDOB      string
	ChildGender   uint8
}

func New(phonePrefix, emailDomain string) *Generator {
	return &Generator{
		phonePrefix: strings.TrimSpace(phonePrefix),
		emailDomain: strings.TrimPrefix(strings.ToLower(strings.TrimSpace(emailDomain)), "@"),
	}
}

func (g *Generator) Generate(runDate time.Time, idx int) Profile {
	faker := gofakeit.New(newSeed(fmt.Sprintf("profile:%s:%d", runDate.Format("20060102"), idx)))

	guardianName := faker.Name()
	childName := faker.FirstName()
	if strings.TrimSpace(guardianName) == "" {
		guardianName = fmt.Sprintf("Daily Guardian %04d", idx+1)
	}
	if strings.TrimSpace(childName) == "" {
		childName = fmt.Sprintf("Daily Child %04d", idx+1)
	}

	phoneSuffix := fmt.Sprintf("%02d%02d%04d", int(runDate.Month()), runDate.Day(), idx+1)
	phone := strings.TrimSpace(g.phonePrefix) + phoneSuffix

	emailLocal := normalizedEmailLocal(guardianName)
	if emailLocal == "" {
		emailLocal = fmt.Sprintf("dailyguardian%04d", idx+1)
	}
	emailDomain := g.emailDomain
	if emailDomain == "" {
		emailDomain = "fangcunmount.com"
	}
	email := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%s_%s_%04d@%s", emailLocal, runDate.Format("20060102"), idx+1, emailDomain)))

	start := runDate.AddDate(-11, 0, 0)
	end := runDate.AddDate(-2, 0, 0)
	childDOB := faker.DateRange(start, end).Format("2006-01-02")

	gender := uint8(1)
	if strings.EqualFold(strings.TrimSpace(faker.Gender()), "female") || idx%2 == 1 {
		gender = 2
	}

	return Profile{
		Index:         idx + 1,
		RunDate:       runDate,
		GuardianName:  guardianName,
		GuardianPhone: phone,
		GuardianEmail: email,
		ChildName:     childName,
		ChildDOB:      childDOB,
		ChildGender:   gender,
	}
}

func normalizedEmailLocal(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	args := pinyin.NewArgs()
	args.Style = pinyin.Normal
	parts := pinyin.LazyPinyin(name, args)
	if len(parts) == 0 {
		parts = []string{name}
	}

	var builder strings.Builder
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
}

func newSeed(seed string) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(seed))
	return hash.Sum64()
}
