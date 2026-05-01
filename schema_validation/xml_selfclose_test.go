package schema_validation

import (
	"strings"
	"testing"

	xj "github.com/basgys/goxml2json"
)

func TestGoxml2jsonSelfClosingElements(t *testing.T) {
	tests := []struct {
		name string
		xml  string
	}{
		{
			name: "nested_self_closing_with_attrs",
			xml:  `<Certificate language="en" name="Test"><IssuedBy><Organization name="CBSE" code="cbse"/></IssuedBy><CertificateData><Examination subject="Math" year="2024"/></CertificateData></Certificate>`,
		},
		{
			name: "top_level_self_closing",
			xml:  `<Exam name="Math" year="2024"/>`,
		},
		{
			name: "nested_elements_not_self_closing",
			xml:  `<Certificate language="en" name="Test"><IssuedBy><Organization name="CBSE" code="cbse"></Organization></IssuedBy><CertificateData><Examination subject="Math" year="2024"></Examination></CertificateData></Certificate>`,
		},
		{
			name: "attrs_as_child_elements",
			xml:  `<Certificate><language>en</language><name>Test</name><IssuedBy><Organization><name>CBSE</name><code>cbse</code></Organization></IssuedBy><CertificateData><Examination><subject>Math</subject><year>2024</year></Examination></CertificateData></Certificate>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf, err := xj.Convert(strings.NewReader(tc.xml))
			if err != nil {
				t.Fatalf("goxml2json error: %v", err)
			}

			t.Logf("xml:  %s", tc.xml)
			t.Logf("json: %s", buf.String())
		})
	}
}
