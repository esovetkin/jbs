// define a catalog of known errors and warnings
package diag

type Code string

type CodeMeta struct {
	Severity Severity
	Owner    string
	Summary  string
}

const (
	CodeE001 Code = "E001"
	CodeE002 Code = "E002"
	CodeE003 Code = "E003"
	CodeE010 Code = "E010"
	CodeE011 Code = "E011"
	CodeE012 Code = "E012"
	CodeE020 Code = "E020"
	CodeE021 Code = "E021"
	CodeE023 Code = "E023"
	CodeE024 Code = "E024"
	CodeE025 Code = "E025"
	CodeE026 Code = "E026"
	CodeE027 Code = "E027"
	CodeE028 Code = "E028"
	CodeE030 Code = "E030"
	CodeE031 Code = "E031"
	CodeE032 Code = "E032"
	CodeE033 Code = "E033"
	CodeE034 Code = "E034"
	CodeE035 Code = "E035"
	CodeE036 Code = "E036"
	CodeE040 Code = "E040"
	CodeE041 Code = "E041"
	CodeE042 Code = "E042"
	CodeE050 Code = "E050"
	CodeE051 Code = "E051"
	CodeE052 Code = "E052"
	CodeE053 Code = "E053"
	CodeE054 Code = "E054"
	CodeE055 Code = "E055"
	CodeE058 Code = "E058"
	CodeE059 Code = "E059"
	CodeE060 Code = "E060"
	CodeE061 Code = "E061"
	CodeE062 Code = "E062"
	CodeE063 Code = "E063"
	CodeE064 Code = "E064"
	CodeE065 Code = "E065"
	CodeE066 Code = "E066"
	CodeE071 Code = "E071"
	CodeE072 Code = "E072"
	CodeE073 Code = "E073"
	CodeE074 Code = "E074"
	CodeE075 Code = "E075"
	CodeE076 Code = "E076"
	CodeE077 Code = "E077"
	CodeE078 Code = "E078"
	CodeE080 Code = "E080"
	CodeE081 Code = "E081"
	CodeE082 Code = "E082"
	CodeE083 Code = "E083"
	CodeE100 Code = "E100"
	CodeE102 Code = "E102"
	CodeE103 Code = "E103"
	CodeE104 Code = "E104"
	CodeE105 Code = "E105"
	CodeE106 Code = "E106"
	CodeE107 Code = "E107"
	CodeE108 Code = "E108"
	CodeE109 Code = "E109"
	CodeE110 Code = "E110"
	CodeE111 Code = "E111"
	CodeE112 Code = "E112"
	CodeE113 Code = "E113"
	CodeE199 Code = "E199"
	CodeE210 Code = "E210"
	CodeE211 Code = "E211"
	CodeE212 Code = "E212"
	CodeE213 Code = "E213"
	CodeE214 Code = "E214"
	CodeE218 Code = "E218"
	CodeE219 Code = "E219"
	CodeE220 Code = "E220"
	CodeE221 Code = "E221"
	CodeE300 Code = "E300"
	CodeE301 Code = "E301"
	CodeE303 Code = "E303"
	CodeE304 Code = "E304"
	CodeE305 Code = "E305"
	CodeE306 Code = "E306"
	CodeE307 Code = "E307"
	CodeE400 Code = "E400"
	CodeE401 Code = "E401"
	CodeE402 Code = "E402"
	CodeE403 Code = "E403"
	CodeE410 Code = "E410"
	CodeE412 Code = "E412"
	CodeE413 Code = "E413"
	CodeE414 Code = "E414"
	CodeE415 Code = "E415"
	CodeE416 Code = "E416"
	CodeE417 Code = "E417"
	CodeE418 Code = "E418"
	CodeE420 Code = "E420"
	CodeE422 Code = "E422"
	CodeE430 Code = "E430"
	CodeE500 Code = "E500"
	CodeE501 Code = "E501"
	CodeE502 Code = "E502"
	CodeE530 Code = "E530"
	CodeE531 Code = "E531"
	CodeE532 Code = "E532"
	CodeE533 Code = "E533"
	CodeE534 Code = "E534"
	CodeE535 Code = "E535"
	CodeE536 Code = "E536"
	CodeE537 Code = "E537"
	CodeW070 Code = "W070"
	CodeW071 Code = "W071"
	CodeW072 Code = "W072"
	CodeW073 Code = "W073"
	CodeW074 Code = "W074"
	CodeW075 Code = "W075"
	CodeW101 Code = "W101"
	CodeW102 Code = "W102"
	CodeW310 Code = "W310"
	CodeW311 Code = "W311"
	CodeW312 Code = "W312"
	CodeW313 Code = "W313"
	CodeW320 Code = "W320"
)

var Catalog = initCatalog()

func initCatalog() map[Code]CodeMeta {
	catalog := make(map[Code]CodeMeta)
	add := func(owner string, severity Severity, summary string, codes ...Code) {
		for _, code := range codes {
			if _, exists := catalog[code]; exists {
				panic("duplicate diagnostic code in catalog: " + string(code))
			}
			catalog[code] = CodeMeta{Severity: severity, Owner: owner, Summary: summary}
		}
	}

	add("lexer", SeverityError, "lexer diagnostic",
		CodeE001, CodeE002, CodeE003,
	)

	add("parser", SeverityError, "parser diagnostic",
		CodeE010, CodeE011, CodeE012,
		CodeE023, CodeE024, CodeE025, CodeE026, CodeE027, CodeE028,
		CodeE030, CodeE031, CodeE032, CodeE033, CodeE034, CodeE035,
		CodeE040, CodeE041,
		CodeE050, CodeE051, CodeE052, CodeE053, CodeE054, CodeE055,
		CodeE058, CodeE059, CodeE060, CodeE061, CodeE062, CodeE063, CodeE064, CodeE065, CodeE066,
		CodeE077,
		CodeE080, CodeE081, CodeE082, CodeE083,
		CodeE416, CodeE417, CodeE418,
	)

	add("eval", SeverityError, "expression or combination evaluation diagnostic",
		CodeE036, CodeE042,
		CodeE100,
		CodeE102, CodeE103, CodeE104, CodeE105, CodeE106, CodeE107, CodeE108, CodeE109, CodeE110, CodeE111, CodeE112, CodeE113,
		CodeE199,
	)

	add("eval", SeverityWarning, "expression or combination evaluation warning",
		CodeW101, CodeW102,
	)

	add("sema", SeverityError, "semantic analysis diagnostic",
		CodeE020, CodeE021,
		CodeE071, CodeE072, CodeE073, CodeE074, CodeE075, CodeE076, CodeE078,
		CodeE210, CodeE211, CodeE212, CodeE213, CodeE214, CodeE218, CodeE219,
		CodeE220, CodeE221,
		CodeE300, CodeE301, CodeE303, CodeE304, CodeE305, CodeE306, CodeE307,
		CodeE400, CodeE401, CodeE402, CodeE403,
		CodeE410, CodeE412, CodeE413, CodeE414, CodeE415,
		CodeE420, CodeE422,
		CodeE430,
	)

	add("sema", SeverityWarning, "semantic analysis warning",
		CodeW070, CodeW071, CodeW072, CodeW073, CodeW074, CodeW075,
		CodeW310, CodeW311, CodeW312, CodeW313,
		CodeW320,
	)

	add("printparam", SeverityError, "printparam diagnostic",
		CodeE500, CodeE501, CodeE502,
	)

	add("imports", SeverityError, "import resolver diagnostic",
		CodeE530, CodeE531, CodeE532, CodeE533, CodeE534, CodeE535, CodeE536, CodeE537,
	)

	return catalog
}

func IsKnownCode(code Code) bool {
	_, ok := Catalog[code]
	return ok
}
