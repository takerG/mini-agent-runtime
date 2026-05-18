package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"mini-agent-runtime/internal/memory"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 闂傚倷娴囧畷鍨叏瀹曞洦濯奸柡灞诲劚閻ょ偓绻濇繝鍌涘櫧闁活厼鐗撻幃妤呮濞戞瑥鏆堥柣搴㈢瀹€鎼佸蓟閿熺姴鐐婇柍杞版缁爼鏌ｉ悩杈╁妽鐟滄澘鍟村﹢渚€姊虹紒姗堜緵闁哥姵鐗犲畷婵嬪Χ婢跺鍘告繛杈剧秮濞佳囧焵椤掍胶绠撻柣锝呭槻閳诲酣骞橀弶鎴炵枀闂備線娼ч¨鈧梻鍕椤㈡瑥顓奸崨顏呮杸闂佸疇妫勫Λ妤呮倶濞嗘挻鐓曢柟鎯ь嚟缁犳﹢鏌?http.RoundTripper闂?
func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type strictPlannerTestTool struct{}

// Name 闂傚倷绀侀幖顐λ囬锕€鐤炬繝濠傜墕閽冪喖鏌曟繛鍨壄?strict planner 婵犵數濮烽弫鎼佸磻閻愬樊鐒芥繛鍡樻惄閺佸嫰鏌涢鐘插姕闁稿骸锕ラ妵鍕冀閵娿儱姣堥梺鍝ュ枎閹虫﹢寮婚弴鐔风窞闁割偅绻傛慨搴ㄦ倵閻㈤潧顣兼俊顐㈠暣瀵鏁愭径濞⑩晠鏌曟径鍫濆姶濞寸娀绠栧鐑樻姜閹峰苯鍘￠梺绋款儍閸婃洟鎮惧畡鎷旂喖姊荤€涙ê濯伴梻浣稿悑娴滀粙宕曢柆宥呯劦妞ゆ巻鍋撶痪缁㈠幘濡叉劙骞掗幘宕囩獮闁硅壈鎻槐鏇㈠礉閻戣姤鈷?
func (strictPlannerTestTool) Name() string {
	return "mock_metric_query"
}

// Description 闂傚倷绀侀幖顐λ囬锕€鐤炬繝濠傜墕閽冪喖鏌曟繛鍨壄?strict planner 婵犵數濮烽弫鎼佸磻閻愬樊鐒芥繛鍡樻惄閺佸嫰鏌涢鐘插姕闁稿骸锕ラ妵鍕冀閵娿儱姣堥梺鍝ュ枎閹虫﹢寮婚弴鐔风窞闁割偅绻傛慨搴ㄦ倵閻㈤潧顣兼俊顐㈠暣瀵鏁愭径濞⑩晠鏌曟径鍫濆姶濞寸娀绠栭弻銈夊垂濞戞瑦鐝氶梺鍝勬湰閻╊垱淇婇幖浣肝ㄦい鏃傚帶婢瑰牓鏌ｉ悢鍝ョ煁濠殿垳鏅禍绋库枎閹惧磭鐤呴柣搴㈢⊕钃卞┑顖氼嚟缁辨帒鈽夊鍡楀壈濠电偛鐗滈崹璺侯潖濞差亜浼犻柛鏇ㄥ亽娴犺偐绱撴担闈涘婵＄偘绮欏畷娲焵?
func (strictPlannerTestTool) Description() string {
	return "query a mock metric value for strict planner tests"
}

// Definition 闂傚倷绀侀幖顐λ囬锕€鐤炬繝濠傜墕閽冪喖鏌曟繛鍨壄?strict planner 婵犵數濮烽弫鎼佸磻閻愬樊鐒芥繛鍡樻惄閺佸嫰鏌涢鐘插姕闁稿骸锕ラ妵鍕冀閵娿儱姣堥梺鍝ュ枎閹虫﹢寮婚弴鐔风窞闁割偅绻傛慨搴ㄦ倵閻㈤潧顣兼俊顐㈠暣瀵?function calling 闂傚倷娴囬褍顫濋敃鍌︾稏濠㈣埖鍔栭崕妤併亜閺傚灝鈷斿☉鎾崇Ч閺岋綁寮崒姘粯閺?
func (t strictPlannerTestTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{
		Type: "function",
		Function: ollama.ToolDescription{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "metric name",
					},
				},
				"required": []string{"name"},
			},
		},
	}
}

// Execute 闂傚倷绀侀幖顐λ囬锕€鐤炬繝濠傜墕閽冪喖鏌曟繛鍨壄?strict planner 婵犵數濮烽弫鎼佸磻閻愬樊鐒芥繛鍡樻惄閺佸嫰鏌涢鐘插姕闁稿骸锕ラ妵鍕冀閵娿儱姣堥梺鍝ュ枎閹虫﹢寮婚弴鐔风窞闁割偅绻傛慨搴ㄦ倵閻㈤潧顣兼俊顐㈠暣瀵鏁愭径濞⑩晠鏌曟径鍫濆姶濞寸姷鍘ч埞鎴︽倷閼碱剚鎲奸梺鍛婃⒐椤ㄥ﹪宕洪埀顒併亜閹烘垵鏋ゆ繛鍏煎姇鑿愰柛銉ｅ妽閵囨繈鏌ㄥ┑鍫濅槐鐎规洖鐖奸垾鏍灳閾忣偉纭€闂侀€炲苯澧剧紓宥呮瀹曟粌鈽夊Ο閿嬬亙闂侀潧绻堥崐鏍偂濞戙垺鍊甸柨婵嗛娴滄粍绻涢崼銏犫枅闁?
func (strictPlannerTestTool) Execute(context.Context, map[string]any) (string, error) {
	return "metric=42", nil
}

// containsMemoryMessage 闂傚倸鍊风粈渚€骞夐敍鍕殰闁搞儺鍓欑壕褰掓煛瀹ュ骸骞栭柦鍐枛閺屾盯濡烽幋鏂夸壕濠碘剝褰冮悧鎾诲蓟濞戙埄鏁冮柕鍫濇噺閻庤櫣绱撴担浠嬪摵闁绘濞€瀵鈽夐姀鐘殿唺闂佺懓鐏濋崯顖炴偩娴犲鈷戦梻鍫熺⊕椤ユ粓鏌涢悤浣哥仯缂侇喗鐟﹀鍕箛閸偅娅嶉梻浣虹帛濮婂綊宕濋崨瀛樻櫇闁稿本绋撻崢鍗炩攽椤旂煫顏呮櫠娴犲鍋╅梺顒€绉甸崐鐢告煕椤垵浜鹃柤鎷屾硶閳ь剝顫夊ú蹇涘磿閻㈡悶鈧線寮撮姀鈩冩珳闂佸憡渚楅崹閬嶅煝瀹€鍕拺閻犲洤寮堕崬澶嬨亜椤愩埄妯€闁硅櫕绻冮妶锝夊礃閵娿劌鐓橀梺璇叉捣閺佸摜娑甸崼鏇炵；闁瑰墽绮崑锟犳煟濡も偓閻楁劗娑甸埀顒佺節?memory system message闂?
func containsMemoryMessage(messages []ollama.Message, want string) bool {
	for _, message := range messages {
		if message.Role == "system" && strings.Contains(message.Content, "Memory context") && strings.Contains(message.Content, want) {
			return true
		}
	}
	return false
}

// TestRunChatLoopSendsConversationHistoryAcrossTurns 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂佽鍨悞锕€顕ラ崟顓涘亾閿濆簼绨界紒鎰仜閳规垿顢欓惌顐簽婢规洟顢橀姀鐘插殤?CLI 濠电姷鏁搁崑鐐差焽濞嗘挸瑙﹂悗锝庡墮閺嬪牏鈧箍鍎卞Λ鏃傛崲閸℃稒鐓欓梻鍌氼嚟椤︼箓鏌涚仦璇插闁哄本鐩崺鍕礃閻愵剛鏆ラ梻浣藉Г閸ㄧ敻藝閹殿喗宕叉繛鎴欏灩缁狅絾绻濋棃娑氬妞ゃ倐鍋撴繝鐢靛仜椤曨參宕㈣楠炲﹪骞囬绛嬫綗闂佸湱鍎ゅú锕€危妤ｅ啯鈷戞慨鐟版搐閻忣喚绱掗鍛仯闁逞屽墴濞佳囧Χ閹间礁绠栭柍鍝勬噺閸嬨劌霉閿濆懎鏆欓柣鎾村灦缁绘繄鍠婂Ο娲绘綉闂佸壊鐓堟禍顏堝春閳ь剚銇勯幒鍡椾壕缂佸墽铏庨崣鍐春閳ь剚銇勯幒鎴濃偓鎼佸储閺夋垟鏀?
func TestRunChatLoopSendsConversationHistoryAcrossTurns(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			answer := "answer one"
			if len(requests) == 2 {
				answer = "answer two"
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"` + answer + `"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		nil,
		strings.NewReader("first\nsecond\n/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Messages), 1; got != want {
		t.Fatalf("first request message count = %d, want %d", got, want)
	}
	if got, want := requests[0].Messages[0], (ollama.Message{Role: "user", Content: "first"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("first request message = %#v, want %#v", got, want)
	}
	if requests[0].Think == nil || !*requests[0].Think {
		t.Fatalf("first request think = %v, want true", requests[0].Think)
	}

	wantHistory := []ollama.Message{
		{Role: "system"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "answer one"},
		{Role: "user", Content: "second"},
	}
	if got := requests[1].Messages; len(got) != len(wantHistory) {
		t.Fatalf("second request message count = %d, want %d", len(got), len(wantHistory))
	}
	for i, want := range wantHistory {
		if want.Content == "" && want.Role == "system" {
			if got := requests[1].Messages[i]; got.Role != "system" || !strings.Contains(got.Content, "Memory context") || !strings.Contains(got.Content, "first") || !strings.Contains(got.Content, "answer one") {
				t.Fatalf("second request memory message = %#v, want memory for first turn", got)
			}
			continue
		}
		if !reflect.DeepEqual(requests[1].Messages[i], want) {
			t.Fatalf("second request message[%d] = %#v, want %#v", i, requests[1].Messages[i], want)
		}
	}
	if got, want := stdout.String(), "answer one\nanswer two\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestRunChatLoopUsesArgsAsFirstMessageThenContinuesReadingStdin 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂佽鍨悞锕€顕ラ崟顖氱疀妞ゆ帒鍞弴鐑嗘富闁靛牆妫涙晶顒勬煠鐟欏嫬绀冩い銈呭濮婄粯鎷呮笟顖滃姼缂備浇顕ч悧鎾崇暦閺囥垹纭€闁绘垵妫楀▓銊ヮ渻閵堝棗绗傜紒鈧笟鈧、姘綇閵婏箑寮垮┑顔筋殔濡绂嶉悙鐢电＜閻庯綆浜峰銉╂煃閽樺妯€妤犵偞顭囬埀顒佺⊕鑿уù鐘灩椤啴濡堕崒娑欑彇缂備緡鍣崹浼粹€﹂崶顒夋晜闁割偒鍋呴弲鈺呮⒑娴兼瑧鍒伴柣顐ｎ殜婵＄兘鍩￠崒婊冨箥婵＄偑鍊栭幐鍡涘礃椤忓啯鍩涚紓鍌氬€烽悞锕傘€冮崱娑欏亱闁绘灏欓弳锕傛煏婢舵盯妾柛搴ｅ枛閺屻劌鈹戦崱妯侯槱闂佸搫妫欐竟鍡欐閹捐纾兼繛鍡樺灩琚ｉ梻浣告啞娓氭宕归柆宥呮辈濞寸厧鐡ㄩ悡鐔兼煟濡搫绾х紒灞惧閵囧嫰寮撮崱妤侇棑濞?stdin闂?
func TestRunChatLoopUsesArgsAsFirstMessageThenContinuesReadingStdin(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"ok"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"from", "args"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 1; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := requests[0].Messages[0], (ollama.Message{Role: "user", Content: "from args"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("first request message = %#v, want %#v", got, want)
	}
}

// TestRunChatLoopUsesConfiguredThinkValue 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?CLI 濠电姷鏁搁崑鐐差焽濞嗘挸瑙﹂悗锝庡墮閺嬪牏鈧箍鍎卞Λ鏃傛崲?think 闂傚倸鍊风粈渚€骞夐敓鐘冲仭闁靛鏅涚壕鍦喐閻楀牆绗掓慨瑙勭叀閺岋綁寮崒銈囧姼濡炪倖娲熸禍鍫曞蓟閿濆憘鐔烘嫚閼碱剛銈烽柣搴㈩問閸犳稑鈻嶉弴鐔衡攳濠电姴娴傞弫宥嗕繆閻愰鍤欏ù婊冨⒔閳ь剝顫夊ú鏍洪悩璇茬；闁圭偓鎯屽Σ褰掑箹濞ｎ剙鐏╅柣婵呭嵆濮婄粯鎷呴崫銉よ檸濡炪倖鍨靛ú銈夋晝閵忋倕绠绘い鏃傛櫕閸?
func TestRunChatLoopUsesConfiguredThinkValue(t *testing.T) {
	var request ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"ok"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		false,
		client,
		[]string{"hello"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if request.Think == nil {
		t.Fatal("request think = nil, want false")
	}
	if *request.Think {
		t.Fatal("request think = true, want false")
	}
}

// TestRunChatLoopSendsToolDefinitions 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂佽鍨悞锕€顕ラ崟顖氱疀妞ゆ帒鍊归悡锝嗕繆閻愵亜鈧牕煤瀹ュ纾婚柟鍓х帛閻撳啰鎲稿鍫濈婵鍩栭崵灞轿旈敐鍛灓闁轰礁顑嗛妵鍕籍閸屾矮澹曢柣搴㈢绾板秶鎹㈠┑瀣棃婵炴垶鐟у顔界箾鏉堝墽鍒板鐟帮躬瀹曠敻寮撮姀銏犲絼闂佹悶鍎滈崟顒€闂紓鍌欑劍閸旀牠銆冩繝鍥ц摕闁绘梻鍎ゅ畷澶愭煟濮橆厽缍戝ù鍏煎姈缁绘盯寮堕幋婵囧€梺鍛婃⒐閻熴儵鎮惧畡鎷旀棃鍩€椤掑嫭鍋╅柨鐔哄Т缁犮儲銇勯弮鍥撻柣锔惧仜閳规垿鎮欓弶鎴犱桓闂佸搫琚崝鎴ｆ濡炪倖鐗楃缓鎸庣瑜版帗鐓曠€光偓閳ь剟宕戦悙鐑樺€块悹鍥梿瑜版帗鏅查柛顐ゅ櫏娴犫晠姊洪崗鑲╂憙闁糕晜鐗楃粚杈ㄧ節閸ヨ埖鏅┑锛勫仩椤曆勭閸撗€鍋撶憴鍕婵炶尙濞€瀹曟垿骞樼紒妯衡偓濠氭煢濡尨绱?
func TestRunChatLoopSendsToolDefinitions(t *testing.T) {
	var request ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"ok"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"what time is it?"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(request.Tools), 3; got != want {
		t.Fatalf("tool count = %d, want %d", got, want)
	}
	toolNames := map[string]bool{}
	for _, tool := range request.Tools {
		toolNames[tool.Function.Name] = true
	}
	for _, want := range []string{"current_time", "calculator", "dangerous_operation"} {
		if !toolNames[want] {
			t.Fatalf("tool names = %v, want %q", toolNames, want)
		}
	}
}

// TestRunChatLoopUsesInjectedToolRegistry 婵犲痉鏉库偓妤佹叏閹绢喗鍎楀〒姘ｅ亾闁?CLI 闂傚倷绀侀幉锟犳偡椤栫偛鍨傛い鏍ㄧ〒椤╂煡鏌熼悜妯虹亶闁哄鐗犻弻鏇熺箾閸喖濮庨悗娈垮枟婵炲﹪寮婚妶鍡欓檮濠㈣泛顦遍惄搴ㄦ⒑娴兼瑧鎮奸柛瀣尵缁瑦绻濋崘銊х獮婵犵數濮抽懗鍫曟儗濡ゅ懏鈷戠紒瀣硶缁犲鏌￠埀顒勬焼瀹ュ懏鐎梺缁樺姇閹碱偆绮堥崘顏佸亾閸忓浜鹃梺鍛婂姧閼靛綊宕戦幘璇茬妞ゆ棁濮ゅ▍鏍倵閸忓浜鹃梺鍛婃处娴滅偟妲愬鑸碘拺缂佸娉曠粻鎵磼婢跺﹦鎽犻悗闈涖偢楠炲洭寮剁捄顭戞Н濠碉紕鍋涢鍛存煀閿濆绀傞柛銉㈡櫇绾捐棄霉閿濆懏鎯堥柡瀣懄閵囧嫰顢曢鍕€嶅銈庝簴閸嬫挸鈹戦埥鍡楃仭妞ゆ垶鐟╁畷銉╁炊椤掍礁鈧敻鏌涢敂璇插箹濞寸姍鍕垫闁绘劕寮堕崰妯尖偓瑙勬礃閿氶柍璇查叄楠炲洭妫冨☉妯炴捇姊?
func TestRunChatLoopUsesInjectedToolRegistry(t *testing.T) {
	var request ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"ok"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}
	registry := tools.NewToolRegistry()
	registry.Register(strictPlannerTestTool{})

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "llama3.2",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"query metric"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Dependencies: ChatLoopDependencies{
			Tools: registry,
		},
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	if got, want := len(request.Tools), 1; got != want {
		t.Fatalf("tool count = %d, want %d", got, want)
	}
	if got, want := request.Tools[0].Function.Name, "mock_metric_query"; got != want {
		t.Fatalf("tool name = %q, want %q", got, want)
	}
}

// TestRunChatLoopExecutesCalculatorToolCallThenAsksModelForFinalAnswer 濠德板€楁慨鎾儗娓氣偓閹焦寰勬繛銏㈠枛閸ㄦ儳鐣烽崶锝呬壕鐎瑰嫰鍋婇崯鍛存煏婢跺牆鍔氱€电増妫冮幃鍦偓锝庝簻閺嗙喖鏌℃担闈涒偓鏍偓鐢靛帶椤繈骞囨担纭呮櫑闂備礁婀遍悷鎶藉幢閳哄倹鏉告俊銈囧Х閸嬬偤宕曢懖鈺傚床婵せ鍋撶€规洘顨婃俊鐑藉Χ閸モ晜鍟村┑鐐村灦閹告挳宕戦幘璇茬骇闁绘垵妫楅弸鐔兼煃瑜滈崜婵嗙暦閻㈤潧鍨濋柕濞炬櫅杩?
func TestRunChatLoopExecutesCalculatorToolCallThenAsksModelForFinalAnswer(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"+","a":2,"b":3}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"2+3=5"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"2+3?"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	secondMessages := requests[1].Messages
	if got, want := len(secondMessages), 3; got != want {
		t.Fatalf("second request message count = %d, want %d", got, want)
	}
	if got, want := secondMessages[1].ToolCalls[0].Function.Name, "calculator"; got != want {
		t.Fatalf("assistant tool call name = %q, want %q", got, want)
	}
	if got, want := secondMessages[2].Role, "tool"; got != want {
		t.Fatalf("tool result role = %q, want %q", got, want)
	}
	if got, want := secondMessages[2].ToolName, "calculator"; got != want {
		t.Fatalf("tool result name = %q, want %q", got, want)
	}
	if got, want := secondMessages[2].Content, "5"; got != want {
		t.Fatalf("tool result content = %q, want %q", got, want)
	}
	if got, want := stdout.String(), "2+3=5\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestRunChatLoopReturnsUnknownToolErrorToModel 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂佽鍨悞锕€顕ラ崟顖氱疀妞ゆ巻鍋撻柣婵嚸—鍐Χ閸涱垳顔囧┑鈽嗗亝閻╊垰鐣烽敐澶婄劦妞ゆ帒瀚崐鐢告偡濞嗗繐顏柛瀣█閺屾稒鎯旈妸銈嗗枤閻庤娲╃紞浣哥暦缁嬭鏃€鎷呴崫鍕辈濠电姷鏁搁崑娑樜涘▎鎴炴殰闁冲搫鎳庨崥褰掓煛瀹ュ海浜圭憸鐗堝笒缁€鍌炴煠濞村娅囬柛娆愮箘缁辨捇宕掑姣欙絾鎱ㄦ繝鍌涜础缂?observation 闂傚倷绀侀幖顐λ囬锕€鐤炬繝濠傜墕閽冪喖鏌曟繛鍨壄婵炲樊浜滈崘鈧銈嗗姂閸婃鏁嶅鍐ｆ斀闁绘劕寮堕ˉ鐐烘煕韫囨挾绠茬紒妤冨枛閸┾偓妞ゆ巻鍋撻柣锝囧厴瀹曠兘顢樺☉妯瑰闂佹寧绻傞崯顐ｄ繆閻戣姤鐓?
func TestRunChatLoopReturnsUnknownToolErrorToModel(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"missing_tool","arguments":{}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"tool unavailable"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"use a missing tool"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	toolMessage := requests[1].Messages[2]
	if got, want := toolMessage.Role, "tool"; got != want {
		t.Fatalf("tool message role = %q, want %q", got, want)
	}
	if got, want := toolMessage.ToolName, "missing_tool"; got != want {
		t.Fatalf("tool message name = %q, want %q", got, want)
	}
	for _, want := range []string{
		"tool error:",
		"code=tool_execution_failed",
		"origin=tools.registry",
		"node_chain=agent.tool_call > tools.registry",
		"detail=unknown tool: missing_tool",
	} {
		if !strings.Contains(toolMessage.Content, want) {
			t.Fatalf("tool message content = %q, want substring %q", toolMessage.Content, want)
		}
	}
	if got, want := stdout.String(), "tool unavailable\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestRunChatLoopReturnsToolExecutionErrorToModel 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂佽鍨悞锕€顕ラ崟顒傜瘈閹肩补鍓濆▍濠囨⒒娓氣偓濞佳囁囬銏犵？闁告鍎愰崵妤呮煛閸愩劎澧涢柣鎾寸懅缁辨帒螖娴ｈ　妲堝┑鈽嗗亽閸欏啫顕ｉ妸锔绢浄閻庯綆鍋€閹锋椽姊洪崷顓х劸婵炲鍏橀獮濠囧幢濡晲绨婚梺鍝勬处閿氶柍褜鍓氶悧婊呭垝濞嗘劕绶炲┑鐘查閹垿姊洪崨濠冨闁告﹢绠栭幃浼村Ψ閿斿墽顔曢梺鐟邦嚟閸嬬偤骞嗛崟顖涚厱婵☆垰婀遍惌娆戔偓娈垮枛椤嘲鐣烽崼鏇炵厴闁诡垎鍌氼棜闂備焦鐪归崹娲敊閺嶎厼缁╁ù鐓庣摠閻撶喖鏌ㄩ弮鍥ㄧ《闁活厽鐟╅弻鐔兼惞椤愶絽鏆楁繛瀵稿缁犳捇骞冨▎鎿冩晞妞ゆ劦鍓涢悾閬嶆婢跺绡€鐟滃秹鎮樺┑瀣劦妞ゆ帊绶″▓姗€鏌涢幒鎾虫诞鐎殿喖澧庨幏鐘诲礈瑜嶉弸娑㈡煕閵婏箑鍔ら柣锝囧厴瀹曞爼鏁愰崨顒€顥氶梻鍌欑閻忔繈顢栭崨顖涱偨闁绘劗鍎ら悡鏇㈡煛閸ャ儱濡兼鐐搭殜閺屾稒绻濋崟顐℃闂?
func TestRunChatLoopReturnsToolExecutionErrorToModel(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"/","a":8,"b":0}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"cannot divide by zero"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"8/0"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	toolMessage := requests[1].Messages[2]
	if got, want := toolMessage.Role, "tool"; got != want {
		t.Fatalf("tool message role = %q, want %q", got, want)
	}
	if got, want := toolMessage.ToolName, "calculator"; got != want {
		t.Fatalf("tool message name = %q, want %q", got, want)
	}
	for _, want := range []string{
		"tool error:",
		"code=tool_execution_failed",
		"origin=tools.calculator",
		"node_chain=agent.tool_call > tools.calculator",
		"detail=division by zero",
	} {
		if !strings.Contains(toolMessage.Content, want) {
			t.Fatalf("tool message content = %q, want substring %q", toolMessage.Content, want)
		}
	}
	if got, want := stdout.String(), "cannot divide by zero\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestRunChatLoopDebugPrintsToolErrorDetails 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?debug 婵犵數濮烽。钘壩ｉ崨鏉戝瀭妞ゅ繐鐗嗛悞鍨亜閹哄棗浜剧紒鍓ц檸閸樻儳鈽夐悽绋跨劦妞ゆ帊鑳剁粻楣冩煛婢跺鐏ラ柟顔藉灩閻ヮ亪寮剁捄銊愶綁鏌￠崨顓犲煟闁轰礁鍟村畷鎺戭潩妲屾牗鏁ら梻鍌欑閹碱偊藝椤栫偛桅婵☆垰鍚嬮～鏇㈡煥濠靛棭妲归柣鎾存礋閻擃偊宕堕妸锔绢槬闂佸憡蓱閹告娊寮婚埄鍐╁闂傚牃鏅涙慨娑㈡⒑瑜版帩妫戝┑鐐╁亾濡炪們鍨虹粙鎴︹€﹂妸鈺佺妞ゆ挾鍠愰蹇曠磽閸屾艾鈧悂宕愰悜鑺ュ€块柨鏇炲€哥粻鏉库攽閻樺磭顣查柛濠傜仛閵囧嫰寮介顫捕缂備讲鍋撶€光偓閸曨剛鍘搁悗骞垮劚妤犲憡绂嶉悙鐑樼厽妞ゆ挾鍋ら崣鍕煛鐏炶鈧繂鐣烽锕€绀嬫い鎰枎娴滈箖鏌涢锝嗙闁?
func TestRunChatLoopDebugPrintsToolErrorDetails(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"missing_tool","arguments":{}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"handled"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "llama3.2",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"use a missing tool"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Debug:       true,
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	got := stderr.String()
	for _, want := range []string{
		"[debug] error:",
		"code=tool_execution_failed",
		"origin=tools.registry",
		"node_chain=agent.tool_call > tools.registry",
		"detail=unknown tool: missing_tool",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr = %q, want substring %q", got, want)
		}
	}
}

// TestRunChatLoopPlannerExecutorModePlansThenExecutesWithTools 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?plan 婵犵數濮烽。钘壩ｉ崨鏉戝瀭妞ゅ繐鐗嗛悞鍨亜閹哄棗浜剧紒鍓ц檸閸樻儳鈽夐悽绋跨劦妞ゆ帊鑳剁粻楣冩煛婢跺鐏ラ柟顔藉灴閺屻倛銇愰幒鏃傛毇閻庤娲忛崹褰掑煡婢跺娼╂い鎾跺仦閻庮參姊绘担鍛婂暈婵炶绠撳畷鎴﹀礋椤栨稑鈧泛鈹戦悩鍙夊闁绘挻娲熼弻鐔兼焽閿曗偓婢т即寮崼銉︹拺闂侇偆鍋涢懟顖涙櫠椤栨搴ㄥ炊椤忓拋浠╅梺鐟扮畭閸ㄨ棄鐣峰鈧、娆撴偂鎼粹€叉喚闂傚倷鑳剁划顖氼潖婵犳艾纾块柕鍫濐槸閻ら箖鏌涘┑鍕姉闁稿鎸鹃幉鎾礋椤掆偓娴犫晠姊虹粙鍧楀弰濞存粏娉涢悾鐑芥晲閸ャ劌纾梺闈涱煭缁犳垿顢旈埡鍛拺闁告稑锕ゆ慨锕€霉濠婂嫮绠炵€殿喖鐤囩粻娑樷槈濞嗘垵骞嶇紓鍌欑劍椤ㄥ棛绮欓幋婢盯顢橀姀鐘靛摋婵炲濮撮鍡涙偂?
func TestRunChatLoopPlannerExecutorModePlansThenExecutesWithTools(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"answer calculation\",\"steps\":[{\"task\":\"calculate 23*19\",\"tool_hint\":\"calculator\"}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			case 2:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"*","a":23,"b":19}}}]},"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"23*19=437"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "qwen3:4b",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"23 * 19?"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Mode:        ModePlan,
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	if got, want := len(requests), 3; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Tools), 0; got != want {
		t.Fatalf("planner request tool count = %d, want %d", got, want)
	}
	if got, want := len(requests[1].Tools), 3; got != want {
		t.Fatalf("executor request tool count = %d, want %d", got, want)
	}
	if !strings.Contains(requests[1].Messages[0].Content, "calculate 23*19") {
		t.Fatalf("executor system prompt = %q, want plan step", requests[1].Messages[0].Content)
	}
	wantOutput := strings.Join([]string{
		"[plan]",
		`1. tool_call calculator {"a":23,"b":19,"op":"*"}`,
		"",
		"[observation]",
		"1. calculator -> 437",
		"",
		"Agent:",
		"23*19=437",
		"",
	}, "\n")
	if got := stdout.String(); got != wantOutput {
		t.Fatalf("stdout = %q, want %q", got, wantOutput)
	}
}

// TestRunChatLoopStrictPlannerExecutorModeExecutesToolsInGo 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?strict-plan 婵犵數濮烽。钘壩ｉ崨鏉戝瀭妞ゅ繐鐗嗛悞鍨亜閹哄棗浜剧紒鍓ц檸閸樻儳鈽夐悽绋跨劦妞ゆ帒瀚埛?Go 闂傚倸鍊烽懗鍫曞磿閻㈢鐤炬繛鎴欏灪閸嬨倝鏌曟繛褍瀚▓浼存⒑閸︻叀妾搁柛鐘崇墱缁濡疯閸犳劙鏌熸潏鎹愮闁逞屽厸缁舵岸鐛€ｎ喗鏅濋柍褜鍓熼幆灞轿旀担鍏哥盎闂佸搫绉查崝搴ㄥ煡婢舵劖鐓冮梺鍨儐椤ュ牓鏌熼鎸庣【闁宠棄顦灒闁兼祴鏅涙慨鐗堜繆閻愵亜鈧牠骞愭繝姘畺闁稿本姘ㄩ弳锔芥叏濡寧纭剧紒鐘茬－閳ь剙绠嶉崕閬嶆偋閸℃稓宓侀柍褜鍓熷缁樻媴閸︻厽鑿囬梺鍛婃煥閻ジ鍩€椤掍礁鍤柛鎾存皑閳ь剟娼ч妶鎼佺嵁閸ヮ剙绾ч悹渚厛濡差垶姊绘担鍛婃儓妞ゆ垶鍔欏畷鎴﹀焵椤掑嫭鐓?
func TestRunChatLoopStrictPlannerExecutorModeExecutesToolsInGo(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"answer calculation\",\"steps\":[{\"type\":\"tool_call\",\"tool_name\":\"calculator\",\"arguments\":{\"op\":\"*\",\"a\":23,\"b\":19}}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"23*19=437"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "qwen3:4b",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"23 * 19?"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Mode:        ModeStrictPlan,
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Tools), 3; got != want {
		t.Fatalf("planner request tool count = %d, want %d", got, want)
	}
	if got, want := len(requests[1].Tools), 0; got != want {
		t.Fatalf("summary request tool count = %d, want %d", got, want)
	}
	if got := stdout.String(); !strings.Contains(got, "[plan]") || !strings.Contains(got, "calculator -> 437") || !strings.Contains(got, "23*19=437") {
		t.Fatalf("stdout = %q, want strict process output", got)
	}
}

// TestRuntimeRunsPlannerExecutorTurnWithSharedDependencies 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?Runtime 闂?planner/executor 婵犵數濮烽弫鎼佸磻閻旂儤宕叉繝闈涱儐閸ゅ嫰鏌涢锝嗙闁绘挻绻堥弻娑滅疀濮橆兛姹楅梺宕囩帛濮婂綊濡甸崟顖氱閻犲搫鎼竟澶娾攽閳╁啫绲绘い顓犲厴瀵鏁愭径娑氱◤濡炪倖宸婚崑鎾斥攽閳ョ偨鍋㈤柡灞剧☉閳诲海浠﹂挊澶嗘嫟缂傚倷绀侀崐鍝ョ矓瑜版帇鈧線寮撮姀鈩冩珫闂佸憡娲﹂崰姘跺煘濞戙垺鈷戦柤濮愬€曢～鎺楁煕閺囥劌骞栭柍顏嗘暬濮?
func TestRuntimeRunsPlannerExecutorTurnWithSharedDependencies(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"answer calculation\",\"steps\":[{\"task\":\"calculate 23*19\",\"tool_hint\":\"calculator\"}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			case 2:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"*","a":23,"b":19}}}]},"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"23*19=437"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:  tools.NewDefaultToolRegistry(nil),
		Trace:  tracing.NewTraceHooks(nil),
		Stdout: &stdout,
	})

	answer, err := runtime.RunPlannerExecutorTurn(context.Background(), "23 * 19?")
	if err != nil {
		t.Fatalf("RunPlannerExecutorTurn returned error: %v", err)
	}
	if got, want := answer, "23*19=437"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	if got, want := len(requests), 3; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got := stdout.String(); !strings.Contains(got, "[plan]") || !strings.Contains(got, "23*19=437") {
		t.Fatalf("stdout = %q, want visible process and final answer", got)
	}
}

// TestRuntimePlannerExecutorTurnUsesMemoryContext 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?plan 婵犵數濮烽。钘壩ｉ崨鏉戝瀭妞ゅ繐鐗嗛悞鍨亜閹哄棗浜剧紒鍓ц檸閸樻儳鈽夐悽绋跨劦妞ゆ帊鑳剁粻楣冩煛婢跺鐏ラ柟顔藉灩閻ヮ亪寮剁捄銊愶絽菐?memory 婵犵數濮烽弫鎼佸磻濞戔懞鍥敇閵忕姷顦悗鍏夊亾闁告洦鍋嗛悡?planner 闂?executor闂?
func TestRuntimePlannerExecutorTurnUsesMemoryContext(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"answer\",\"steps\":[{\"task\":\"answer with memory\"}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"memory-aware answer"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	query := memory.Query{UserID: "u1", SessionID: "s1"}
	manager := memory.NewManager(memory.NewWindowMemory(memory.WindowMemoryOptions{Scope: memory.ScopeSession, MaxTurns: 2}))
	if err := manager.AppendTurn(context.Background(), query, memory.Turn{User: "remember color", Assistant: "blue"}); err != nil {
		t.Fatalf("AppendTurn returned error: %v", err)
	}
	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:       tools.NewDefaultToolRegistry(nil),
		Trace:       tracing.NewTraceHooks(nil),
		Stdout:      &stdout,
		Memory:      manager,
		MemoryQuery: query,
	})

	answer, err := runtime.RunPlannerExecutorTurn(context.Background(), "what color?")
	if err != nil {
		t.Fatalf("RunPlannerExecutorTurn returned error: %v", err)
	}
	if got, want := answer, "memory-aware answer"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	for index, request := range requests {
		if !containsMemoryMessage(request.Messages, "remember color") {
			t.Fatalf("request[%d] messages = %#v, want memory context", index, request.Messages)
		}
	}
}

// TestRuntimeRunsStrictPlannerExecutorTurnWithoutModelToolCalls 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?strict-plan 婵犵數濮烽弫鎼佸磻閻旂儤宕叉繝闈涱儐閸ゅ嫰鏌涢锝嗙闁绘挻绻堥弻娑滅疀濮橆兛姹楅梺宕囩帛濮婂鍩€椤掆偓缁犲秹宕曟潏鈹惧亾濮樼厧鏋ょ紒顕嗙到铻栭柛娑卞枛娴狀垱绻涙潏鍓у埌鐎殿喛娉涢埢宥夊幢濞戞瑥娈欓梺鍐叉惈閹冲繘鎮￠弴銏＄厪濠电偛鐏濋崝鎾煕鎼粹剝鎹ｇ紒杈ㄥ浮婵℃悂濮€閳╁啰褰呴梻浣哥枃椤曆囨煀閿濆鍨傚Δ锝呭暙缁€鍐煕濞嗗浚妲洪柍褜鍓﹂崑濠囧蓟閿濆棙鍎熼柕鍫濆缂嶅牓姊洪悡搴㈣础闁稿鎸鹃埀顒勬涧閵堟悂鐛崶顒€绾ч悹渚厛濡差垶姊绘担鍦菇闁稿鍊濆畷褰掑礃濞村鐏侀梺鍓插亝濞叉﹢鎮￠悢鎼炰簻妞ゆ劦鍋勬晶顔尖攽閳╁啯鍊愰柡?
func TestRuntimeRunsStrictPlannerExecutorTurnWithoutModelToolCalls(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"make record\",\"steps\":[{\"type\":\"tool_call\",\"tool_name\":\"calculator\",\"arguments\":{\"a\":23,\"b\":19,\"op\":\"*\"}},{\"type\":\"tool_call\",\"tool_name\":\"current_time\",\"arguments\":{}}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"calculation result is 437, current time is 2026-05-09 10:20:30 CST."}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools: tools.NewDefaultToolRegistry(func() time.Time {
			return time.Date(2026, 5, 9, 10, 20, 30, 0, time.FixedZone("CST", 8*60*60))
		}),
		Trace:  tracing.NewTraceHooks(nil),
		Stdout: &stdout,
	})

	answer, err := runtime.RunStrictPlannerExecutorTurn(context.Background(), "calculate 23 * 19 and get current time")
	if err != nil {
		t.Fatalf("RunStrictPlannerExecutorTurn returned error: %v", err)
	}

	if got, want := answer, "calculation result is 437, current time is 2026-05-09 10:20:30 CST."; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Tools), 3; got != want {
		t.Fatalf("strict planner request tool count = %d, want %d", got, want)
	}
	if got, want := len(requests[1].Tools), 0; got != want {
		t.Fatalf("strict summary request tool count = %d, want %d", got, want)
	}
	if !strings.Contains(requests[1].Messages[0].Content, "observations") {
		t.Fatalf("summary system prompt = %q, want observations", requests[1].Messages[0].Content)
	}

	wantOutput := strings.Join([]string{
		"[plan]",
		`1. tool_call calculator {"a":23,"b":19,"op":"*"}`,
		"2. tool_call current_time {}",
		"",
		"[observation]",
		"1. calculator -> 437",
		"2. current_time -> 2026-05-09 10:20:30 CST",
		"",
		"Agent:",
		"calculation result is 437, current time is 2026-05-09 10:20:30 CST.",
	}, "\n")
	if got := stdout.String(); got != wantOutput {
		t.Fatalf("stdout = %q, want %q", got, wantOutput)
	}
}

// TestRuntimeStrictPlannerPassesRegisteredToolsToPlanner 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?strict planner 闂傚倷娴囧畷鍨叏閺夋嚚娲敇閵忕姷鍝楅梻渚囧墮缁夌敻宕曢幋锔界厽婵°倐鍋撻柣妤€妫濋、姘綇閵婏箑寮垮┑顔筋殔濡绂嶅鍫熺厓閻犲洩灏欑粻濠氭煛瀹€瀣М闁挎繄鍋ら、妤呭焵椤掍椒绻嗗ù鐘差儐閻撴稑霉閿濆懏鎯堝ù鐘轰含閳ь剝顫夊ú姗€鏁嬮梺浼欑稻缁诲牆鐣峰鈧弫鍌炴偡妫颁礁顥氶梻浣告啞閸斿繘寮插┑瀣厺闁哄啠鍋撻柕鍥у楠炴鎹勯惄鎺炵秮閺岋綁鏁愭径澶嬪枤闂佸搫鏈惄顖氼嚕椤掑倹瀚氶柟缁樺俯閻庢挳姊绘笟鈧埀顒傚仜閼活垶宕㈤崫銉х＜闁逞屽墯閹峰懘鎮烽柇锕€娈奸梻浣告贡婢ф顭垮Ο鑲╃焼闁稿本澹曢崑鎾绘偡閺夋浠炬繝銏㈡嚀濡繂顕ｉ幎钘夌骇閹煎瓨鎸婚～宥夋⒑閸濆嫬鈧綊顢栧▎寰綁宕奸弴鐔哄帗?
func TestRuntimeStrictPlannerPassesRegisteredToolsToPlanner(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"query metric\",\"steps\":[{\"type\":\"tool_call\",\"tool_name\":\"mock_metric_query\",\"arguments\":{\"name\":\"latency\"}}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"metric=42"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	registry := tools.NewToolRegistry()
	registry.Register(strictPlannerTestTool{})

	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:  registry,
		Trace:  tracing.NewTraceHooks(nil),
		Stdout: &stdout,
	})

	answer, err := runtime.RunStrictPlannerExecutorTurn(context.Background(), "query latency metric")
	if err != nil {
		t.Fatalf("RunStrictPlannerExecutorTurn returned error: %v", err)
	}

	if got, want := answer, "metric=42"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Tools), 1; got != want {
		t.Fatalf("strict planner request tool count = %d, want %d", got, want)
	}
	if got, want := requests[0].Tools[0].Function.Name, "mock_metric_query"; got != want {
		t.Fatalf("strict planner tool name = %q, want %q", got, want)
	}
	if got, want := requests[0].Messages[1].Content, "query latency metric"; got != want {
		t.Fatalf("strict planner user message = %q, want %q", got, want)
	}
	for _, forbidden := range []string{"available_tools", "mock_metric_query", "parameters"} {
		if strings.Contains(requests[0].Messages[1].Content, forbidden) {
			t.Fatalf("strict planner user message = %q, must not duplicate tool schema %q", requests[0].Messages[1].Content, forbidden)
		}
	}
	plannerPrompt := requests[0].Messages[0].Content
	for _, forbidden := range []string{"Use current_time", "Use calculator", "Available tools are current_time and calculator", "available_tools"} {
		if strings.Contains(plannerPrompt, forbidden) {
			t.Fatalf("strict planner prompt = %q, must not contain hard-coded tool hint %q", plannerPrompt, forbidden)
		}
	}
	if got, want := len(requests[1].Tools), 0; got != want {
		t.Fatalf("strict summary request tool count = %d, want %d", got, want)
	}
	if !strings.Contains(stdout.String(), "mock_metric_query -> metric=42") {
		t.Fatalf("stdout = %q, want dynamic tool observation", stdout.String())
	}
}

// TestRuntimeStrictPlannerUsesMemoryContext 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?strict-plan 婵犵數濮烽。钘壩ｉ崨鏉戝瀭妞ゅ繐鐗嗛悞鍨亜閹哄棗浜剧紒鍓ц檸閸樻儳鈽夐悽绋跨劦妞ゆ帊鑳剁粻楣冩煛婢跺鐏ラ柟顔藉灩閻ヮ亪寮剁捄銊愶絽菐?memory 婵犵數濮烽弫鎼佸磻濞戔懞鍥敇閵忕姷顦悗鍏夊亾闁告洦鍋嗛悡?planner 闂?responder闂?
func TestRuntimeStrictPlannerUsesMemoryContext(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"unsupported request\",\"steps\":[]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"memory-aware strict answer"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	query := memory.Query{UserID: "u1", SessionID: "s1"}
	manager := memory.NewManager(memory.NewSummaryMemory(memory.SummaryMemoryOptions{Scope: memory.ScopeUser}))
	if err := manager.AppendTurn(context.Background(), query, memory.Turn{User: "remember city", Assistant: "Tokyo"}); err != nil {
		t.Fatalf("AppendTurn returned error: %v", err)
	}
	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:       tools.NewDefaultToolRegistry(nil),
		Trace:       tracing.NewTraceHooks(nil),
		Stdout:      &stdout,
		Memory:      manager,
		MemoryQuery: query,
	})

	answer, err := runtime.RunStrictPlannerExecutorTurn(context.Background(), "what city?")
	if err != nil {
		t.Fatalf("RunStrictPlannerExecutorTurn returned error: %v", err)
	}
	if got, want := answer, "memory-aware strict answer"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	for index, request := range requests {
		if !containsMemoryMessage(request.Messages, "remember city") {
			t.Fatalf("request[%d] messages = %#v, want memory context", index, request.Messages)
		}
	}
}

// TestRuntimeStrictPlannerExecutorFeedsToolErrorsIntoObservations 濠电姴鐥夐弶搴撳亾濡や焦鍙忛柟缁㈠枟閸庢銆掑锝呬壕闂?strict-plan 闂備浇顕у锕傦綖婢舵劕绠栭柛顐ｆ礀绾惧潡姊洪鈧粔鎾儗濡ゅ懏鐓曠憸搴ㄣ€冮崱娑欏亗闁靛濡囩粻楣冩煕閳╁厾顏呯┍椤栫偞鐓曢柕鍫濇婢ь亪妫佹径鎰厱闊洦绋掗敍鐔访瑰┃鍨伄缂佽鲸甯炵槐鎺戭潨閸絺鍋撶捄銊㈠亾?observation闂?
func TestRuntimeStrictPlannerExecutorFeedsToolErrorsIntoObservations(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"bad tool\",\"steps\":[{\"type\":\"tool_call\",\"tool_name\":\"missing_tool\",\"arguments\":{}}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}

			if !strings.Contains(requests[1].Messages[0].Content, "tool error:") {
				t.Fatalf("summary prompt = %q, want tool error observation", requests[1].Messages[0].Content)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"tool unavailable"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:  tools.NewDefaultToolRegistry(nil),
		Trace:  tracing.NewTraceHooks(nil),
		Stdout: &stdout,
	})

	_, err := runtime.RunStrictPlannerExecutorTurn(context.Background(), "use missing tool")
	if err != nil {
		t.Fatalf("RunStrictPlannerExecutorTurn returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "unknown tool: missing_tool") {
		t.Fatalf("stdout = %q, want unknown tool observation", stdout.String())
	}
}
