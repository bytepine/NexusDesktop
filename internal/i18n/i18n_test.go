// Copyright byteyang. All Rights Reserved.

package i18n

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"zh-CN":       LangZhCN,
		"zh_CN":       LangZhCN,
		"zh-Hans":     LangZhCN,
		"zh":          LangZhCN,
		"zh_CN.UTF-8": LangZhCN,
		"en-US":       LangEn,
		"en":          LangEn,
		"ja-JP":       LangEn,
		"":            LangEn,
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestResolveAndT(t *testing.T) {
	if Resolve(LangEn) != LangEn {
		t.Fatal("Resolve(en)")
	}
	if Resolve(LangZhCN) != LangZhCN {
		t.Fatal("Resolve(zh-CN)")
	}
	Apply(LangEn)
	if T("tray.quit") != "Quit" {
		t.Fatalf("en quit=%q", T("tray.quit"))
	}
	Apply(LangZhCN)
	if T("tray.quit") != "退出" {
		t.Fatalf("zh quit=%q", T("tray.quit"))
	}
}

func TestPrefFromSelect(t *testing.T) {
	if PrefFromSelect("简体中文") != LangZhCN {
		t.Fatal("zh")
	}
	if PrefFromSelect("English") != LangEn {
		t.Fatal("en")
	}
	if PrefFromSelect("Follow system") != LangAuto {
		t.Fatal("auto")
	}
}

func TestCatalogKeysMatch(t *testing.T) {
	zh, en := catalogs[LangZhCN], catalogs[LangEn]
	if len(zh) != len(en) {
		t.Fatalf("zh=%d en=%d keys", len(zh), len(en))
	}
	for k := range zh {
		if _, ok := en[k]; !ok {
			t.Errorf("en missing key %s", k)
		}
	}
	for k := range en {
		if _, ok := zh[k]; !ok {
			t.Errorf("zh missing key %s", k)
		}
	}
}
