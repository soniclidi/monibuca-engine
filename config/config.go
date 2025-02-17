package config

import (
	"net"
	"net/http"
	"reflect"
	"strings"

	"go.uber.org/zap"
	"m7s.live/engine/v4/log"
)

type Config map[string]any

type Plugin interface {
	// 可能的入参类型：FirstConfig 第一次初始化配置，Config 后续配置更新，SE系列（StateEvent）流状态变化事件
	OnEvent(any)
}

type TCPPlugin interface {
	Plugin
	ServeTCP(*net.TCPConn)
}

type HTTPPlugin interface {
	Plugin
	http.Handler
}

func (config Config) Unmarshal(s any) {
	defer func() {
		if err := recover(); err != nil {
			log.Error("Unmarshal error:", err)
		}
	}()
	if s == nil {
		return
	}
	var el reflect.Value
	if v, ok := s.(reflect.Value); ok {
		el = v
	} else {
		el = reflect.ValueOf(s)
	}
	if el.Kind() == reflect.Pointer {
		el = el.Elem()
	}
	t := el.Type()
	if t.Kind() == reflect.Map {
		for k, v := range config {
			el.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v).Convert(t.Elem()))
		}
		return
	}
	//字段映射，小写对应的大写
	nameMap := make(map[string]string)
	for i, j := 0, t.NumField(); i < j; i++ {
		name := t.Field(i).Name
		nameMap[strings.ToLower(name)] = name
	}
	for k, v := range config {
		name, ok := nameMap[k]
		if !ok {
			log.Error("no config named:", k)
			continue
		}
		// 需要被写入的字段
		fv := el.FieldByName(name)
		ft := fv.Type()
		// 先处理值是数组的情况
		if value := reflect.ValueOf(v); value.Kind() == reflect.Slice {
			l := value.Len()
			s := reflect.MakeSlice(ft, l, value.Cap())
			for i := 0; i < l; i++ {
				fv := value.Index(i)
				if ft == reflect.TypeOf(config) {
					fv.FieldByName("Unmarshal").Call([]reflect.Value{fv})
				} else {
					item := s.Index(i)
					if fv.Kind() == reflect.Interface {
						item.Set(reflect.ValueOf(fv.Interface()).Convert(item.Type()))
					} else {
						item.Set(fv)
					}
				}
			}
			fv.Set(s)
		} else if child, ok := v.(Config); ok { //然后处理值是递归情况（map)
			if fv.Kind() == reflect.Map {
				if fv.IsNil() {
					fv.Set(reflect.MakeMap(ft))
				}
			}
			child.Unmarshal(fv)
		} else {
			switch fv.Kind() {
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				fv.SetUint(uint64(value.Int()))
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				fv.SetInt(value.Int())
			case reflect.Float32, reflect.Float64:
				fv.SetFloat(value.Float())
			case reflect.Slice: //值是单值，但类型是数组，默认解析为一个元素的数组
				s := reflect.MakeSlice(ft, 1, 1)
				s.Index(0).Set(value)
				fv.Set(s)
			default:
				fv.Set(value)
			}
		}
	}
}

// 覆盖配置
func (config Config) Assign(source Config) {
	for k, v := range source {
		switch m := config[k].(type) {
		case Config:
			switch vv := v.(type) {
			case Config:
				m.Assign(vv)
			case map[string]any:
				m.Assign(Config(vv))
			}
		default:
			config[k] = v
		}
	}
}

// 合并配置，不覆盖
func (config Config) Merge(source Config) {
	for k, v := range source {
		if _, ok := config[k]; !ok {
			switch m := config[k].(type) {
			case Config:
				m.Merge(v.(Config))
			default:
				log.Debug("merge", zap.String("k", k), zap.Any("v", v))
				config[k] = v
			}
		} else {
			log.Debug("exist", zap.String("k", k))
		}
	}
}

func (config *Config) Set(key string, value any) {
	if *config == nil {
		*config = Config{strings.ToLower(key): value}
	} else {
		(*config)[strings.ToLower(key)] = value
	}
}

func (config Config) Get(key string) any {
	v, _ := config[strings.ToLower(key)]
	return v
}

func (config Config) Has(key string) (ok bool) {
	_, ok = config[strings.ToLower(key)]
	return
}

func (config Config) HasChild(key string) (ok bool) {
	_, ok = config[strings.ToLower(key)].(Config)
	return ok
}

func (config Config) GetChild(key string) Config {
	if v, ok := config[strings.ToLower(key)]; ok {
		return v.(Config)
	}
	return nil
}

func Struct2Config(s any) (config Config) {
	config = make(Config)
	var t reflect.Type
	var v reflect.Value
	if vv, ok := s.(reflect.Value); ok {
		v = vv
		t = vv.Type()
	} else {
		t = reflect.TypeOf(s)
		v = reflect.ValueOf(s)
		if t.Kind() == reflect.Pointer {
			v = v.Elem()
			t = t.Elem()
		}
	}
	for i, j := 0, t.NumField(); i < j; i++ {
		ft := t.Field(i)
		if !ft.IsExported() {
			continue
		}
		name := strings.ToLower(ft.Name)
		switch ft.Type.Kind() {
		case reflect.Struct:
			config[name] = Struct2Config(v.Field(i))
		case reflect.Slice:
			fallthrough
		default:
			reflect.ValueOf(config).SetMapIndex(reflect.ValueOf(name), v.Field(i))
		}
	}
	return
}
