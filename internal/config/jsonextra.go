package config

import (
	"encoding/json"
	"reflect"
	"strings"
)

type fileFields File
type settingsFields Settings
type profileFields Profile

func (f File) MarshalJSON() ([]byte, error) {
	return marshalWithExtra(fileFields(f), f.extra)
}

func (f *File) UnmarshalJSON(data []byte) error {
	fields := fileFields(*f)
	extra, err := unmarshalWithExtra(data, &fields)
	if err != nil {
		return err
	}
	*f = File(fields)
	f.extra = extra
	return nil
}

func (s Settings) MarshalJSON() ([]byte, error) {
	return marshalWithExtra(settingsFields(s), s.extra)
}

func (s *Settings) UnmarshalJSON(data []byte) error {
	fields := settingsFields(*s)
	extra, err := unmarshalWithExtra(data, &fields)
	if err != nil {
		return err
	}
	*s = Settings(fields)
	s.extra = extra
	return nil
}

func (p Profile) MarshalJSON() ([]byte, error) {
	return marshalWithExtra(profileFields(p), p.extra)
}

func (p *Profile) UnmarshalJSON(data []byte) error {
	fields := profileFields(*p)
	extra, err := unmarshalWithExtra(data, &fields)
	if err != nil {
		return err
	}
	*p = Profile(fields)
	p.extra = extra
	return nil
}

func marshalWithExtra(value any, extra map[string]json.RawMessage) ([]byte, error) {
	content, err := json.Marshal(value)
	if err != nil || len(extra) == 0 {
		return content, err
	}

	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal(content, &merged); err != nil {
		return nil, err
	}
	for key, raw := range extra {
		if _, known := merged[key]; !known {
			merged[key] = raw
		}
	}

	return json.Marshal(merged)
}

func unmarshalWithExtra(data []byte, target any) (map[string]json.RawMessage, error) {
	if err := json.Unmarshal(data, target); err != nil {
		return nil, err
	}

	all := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}

	fields := reflect.TypeOf(target).Elem()
	for index := 0; index < fields.NumField(); index++ {
		name, _, _ := strings.Cut(fields.Field(index).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			delete(all, name)
		}
	}

	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}
