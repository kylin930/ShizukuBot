// ShizukuBot Plugin Nodes - LiteGraph 自定义节点（中文版）

// ==================== 覆盖 LiteGraph 默认菜单为中文 ====================
(function() {
    LGraphCanvas.prototype.getCanvasMenuOptions = function() {
        return [
            {
                content: "添加节点",
                has_submenu: true,
                callback: LGraphCanvas.onMenuAdd
            },
            {
                content: "添加分组",
                callback: LGraphCanvas.onGroupAdd
            },
            null,
            {
                content: "刷新画布",
                callback: function() {
                    if (this.graph) this.graph.change();
                }.bind(this)
            }
        ];
    };

    LGraphCanvas.prototype.getNodeMenuOptions = function(node) {
        return [
            { content: "重命名", callback: LGraphCanvas.onShowPropertyEditor },
            { content: "复制", callback: LGraphCanvas.onMenuNodeClone },
            null,
            { content: "颜色", has_submenu: true, callback: LGraphCanvas.onMenuNodeColors },
            { content: "形状", has_submenu: true, callback: LGraphCanvas.onMenuNodeShapes },
            null,
            { content: "删除", callback: LGraphCanvas.onMenuNodeRemove }
        ];
    };

    LGraphCanvas.onMenuNodeColors = function(value, options, event, menu, node) {
        new LiteGraph.ContextMenu([
            { content: "红色", value: "#a33" },
            { content: "绿色", value: "#3a3" },
            { content: "蓝色", value: "#33a" },
            { content: "黄色", value: "#aa3" },
            { content: "紫色", value: "#a3a" },
            { content: "橙色", value: "#a73" },
            { content: "白色", value: "#aaa" },
            { content: "黑色", value: "#333" }
        ], { event: event, parentMenu: menu, callback: function(v) {
            if (!node) return;
            node.color = v.value;
            node.graph.change();
        }});
    };

    LGraphCanvas.onMenuNodeShapes = function(value, options, event, menu, node) {
        new LiteGraph.ContextMenu([
            { content: "方形", value: "box" },
            { content: "圆角", value: "round" },
            { content: "卡片", value: "card" }
        ], { event: event, parentMenu: menu, callback: function(v) {
            if (!node) return;
            node.shape = v.value;
            node.graph.change();
        }});
    };

    var createNodeCallback = function(canvas, graph, callback) {
        return function(v) {
            if (!v || !v.value) return;
            graph.beforeChange();
            var n = LiteGraph.createNode(v.value);
            if (n) {
                n.pos = canvas.convertCanvasToOffset([
                    canvas.last_mouse_position[0],
                    canvas.last_mouse_position[1]
                ]);
                graph.add(n);
            }
            if (callback) callback(n);
            graph.afterChange();
        };
    };

    // 内置菜单（变量形式，方便追加自定义节点）
    var builtinMenuNodes = [
        null,
        { content: "事件", has_submenu: true, submenu: { title: "事件", options: [
                { content: "收到群消息", value: "event/on_group_message" },
                { content: "收到私聊消息", value: "event/on_c2c_message" },
                { content: "收到指令", value: "event/on_command" },
                { content: "收到频道消息", value: "event/on_guild_message" },
                { content: "收到频道私信", value: "event/on_direct_message" },
                { content: "好友添加/删除", value: "event/on_c2c_friend" },
                { content: "互动事件", value: "event/on_interaction" },
                { content: "订阅消息状态变更", value: "event/on_subscribe_msg_status" },
                { content: "群管理事件", value: "event/on_group_manage" },
                { content: "C2C管理事件", value: "event/on_c2c_manage" }
            ]}},
            null,
            { content: "消息处理", has_submenu: true, submenu: { title: "消息处理", options: [
                { content: "解析指令参数", value: "message/parse_command" },
                { content: "提取@用户ID", value: "message/extract_mention_user" },
                { content: "提取消息中的图片", value: "message/parse_images" }
            ]}},
            null,
            { content: "发送消息", has_submenu: true, submenu: { title: "发送消息", options: [
                { content: "回复消息", value: "action/send_message" },
                { content: "发送群消息", value: "action/send_group_message" },
                { content: "发送私聊消息", value: "action/send_c2c_message" },
                { content: "发送图片", value: "action/send_image" },
                { content: "发送图文消息", value: "action/send_rich_media" },
                { content: "发送Markdown", value: "action/send_markdown" },
                { content: "发送按钮消息", value: "action/send_buttons" },
                { content: "Markdown+按钮", value: "action/send_markdown_with_buttons" },
                { content: "发送视频", value: "action/send_video" },
                { content: "撤回消息", value: "action/recall_message" }
            ]}},
            null,
            { content: "HTML渲染", has_submenu: true, submenu: { title: "HTML渲染", options: [
                { content: "HTML转图片", value: "render/html_to_image" },
                { content: "HTML模板", value: "render/html_template" }
            ]}},
            null,
            { content: "文件操作", has_submenu: true, submenu: { title: "文件操作", options: [
                { content: "读取文件", value: "file/read" },
                { content: "写入文件", value: "file/write" }
            ]}},
            null,
            { content: "条件判断", has_submenu: true, submenu: { title: "条件判断", options: [
                { content: "文本包含", value: "condition/text_contains" },
                { content: "文本相等", value: "condition/text_equals" },
                { content: "正则匹配", value: "condition/text_matches" },
                { content: "条件分支", value: "logic/branch" },
                { content: "数字比较", value: "condition/number_compare" },
                { content: "是否为空", value: "condition/is_empty" },
                { content: "逻辑与", value: "logic/and" },
                { content: "逻辑或", value: "logic/or" },
                { content: "逻辑非", value: "logic/not" }
            ]}},
            null,
            { content: "文本处理", has_submenu: true, submenu: { title: "文本处理", options: [
                { content: "文本模板", value: "text/template" },
                { content: "文本拼接", value: "text/concat" },
                { content: "文本截取", value: "text/substring" },
                { content: "文本替换", value: "text/replace" },
                { content: "文本分割", value: "text/split" },
                { content: "文本长度", value: "text/length" },
                { content: "转大写", value: "text/to_upper" },
                { content: "转小写", value: "text/to_lower" },
                { content: "去空格", value: "text/trim" },
                { content: "查找文本", value: "text/find" },
                { content: "获取字符", value: "text/char_at" },
                { content: "正则替换", value: "text/regex_replace" }
            ]}},
            null,
            { content: "数学运算", has_submenu: true, submenu: { title: "数学运算", options: [
                { content: "数字常量", value: "math/number" },
                { content: "加法", value: "math/add" },
                { content: "减法", value: "math/subtract" },
                { content: "乘法", value: "math/multiply" },
                { content: "除法", value: "math/divide" },
                { content: "取模", value: "math/modulo" },
                { content: "随机数", value: "math/random" },
                { content: "四舍五入", value: "math/round" },
                { content: "向下取整", value: "math/floor" },
                { content: "向上取整", value: "math/ceil" },
                { content: "绝对值", value: "math/abs" },
                { content: "类型转换", value: "math/type_convert" },
                { content: "转换为数字", value: "math/to_number" },
                { content: "平方根", value: "math/sqrt" },
                { content: "正弦", value: "math/sin" },
                { content: "π常量", value: "math/pi" },
                { content: "判断偶数", value: "math/is_even" },
                { content: "列表求和", value: "math/sum" },
                { content: "限制范围", value: "math/clamp" },
                { content: "随机小数", value: "math/random_float" }
            ]}},
            null,
            { content: "网络请求", has_submenu: true, submenu: { title: "网络请求", options: [
                { content: "HTTP请求", value: "http/request" },
                { content: "JSON解析", value: "json/parse" },
                { content: "JSON生成", value: "json/generate" },
                { content: "URL编码", value: "http/url_encode" },
                { content: "URL解码", value: "http/url_decode" }
            ]}},
            null,
            { content: "变量", has_submenu: true, submenu: { title: "变量", options: [
                { content: "设置变量", value: "variable/set" },
                { content: "获取变量", value: "variable/get" },
                { content: "删除变量", value: "variable/delete" },
                { content: "常量", value: "variable/constant" }
            ]}},
            null,
            { content: "时间", has_submenu: true, submenu: { title: "时间", options: [
                { content: "当前时间", value: "time/now" },
                { content: "格式化时间", value: "time/format" },
                { content: "时间计算", value: "time/calculate" },
                { content: "延时执行", value: "time/delay" },
                { content: "13位时间戳", value: "time/timestamp" },
                { content: "日期时间格式化", value: "time/format_custom" }
            ]}},
            null,
            { content: "代码", has_submenu: true, submenu: { title: "代码", options: [
                { content: "Go代码", value: "code/go" }
            ]}},
            null,
            { content: "日志", has_submenu: true, submenu: { title: "日志", options: [
                { content: "输出日志", value: "log/debug" }
            ]}},
            null,
            { content: "指令", has_submenu: true, submenu: { title: "指令", options: [
                { content: "创建指令", value: "command/create" },
                { content: "权限检查", value: "command/permission" }
            ]}},
            null,
            { content: "上下文信息", has_submenu: true, submenu: { title: "上下文信息", options: [
                { content: "获取发送者ID", value: "context/get_user_id" },
                { content: "获取群聊ID", value: "context/get_group_id" },
                { content: "获取频道ID", value: "context/get_channel_id" },
                { content: "获取GuildID", value: "context/get_guild_id" },
                { content: "获取消息ID", value: "context/get_message_id" },
                { content: "获取消息来源", value: "context/get_source" },
                { content: "当前插件ID", value: "context/get_plugin_id" }
            ]}},
            null,
            { content: "列表", has_submenu: true, submenu: { title: "列表", options: [
                { content: "创建空列表", value: "list/create_empty" },
                { content: "创建列表", value: "list/create" },
                { content: "添加重复项", value: "list/add_repeat" },
                { content: "列表长度", value: "list/length" },
                { content: "列表是否为空", value: "list/is_empty" },
                { content: "查找元素", value: "list/find" },
                { content: "获取元素", value: "list/get" },
                { content: "设置元素", value: "list/set" },
                { content: "获取子列表", value: "list/sublist" },
                { content: "从文本制作列表", value: "list/from_text" },
                { content: "排序", value: "list/sort" },
                { content: "删除列表", value: "list/delete" }
            ]}}
    ];

    // 自定义节点菜单（全部放进"自定义"分类）
    var customNodeMenu = { "自定义": [] };

    // 注册自定义节点到菜单
    function registerCustomNodeMenu(def) {
        customNodeMenu["自定义"].push({
            content: def.title || def.type,
            value: def.type
        });
    }

    // 构建最终菜单
    function buildMenu(cb) {
        // 深拷贝内置菜单，给每个子菜单加上 callback
        var menu = JSON.parse(JSON.stringify(builtinMenuNodes));
        function applyCallback(arr) {
            for (var i = 0; i < arr.length; i++) {
                if (arr[i] && arr[i].submenu) {
                    arr[i].callback = cb;
                    applyCallback(arr[i].submenu.options);
                }
            }
        }
        applyCallback(menu);

        // 追加自定义节点分类
        var cats = Object.keys(customNodeMenu);
        if (cats.length > 0) {
            menu.push(null);
            for (var i = 0; i < cats.length; i++) {
                var cat = cats[i];
                menu.push({
                    content: cat,
                    has_submenu: true,
                    submenu: {
                        title: cat,
                        options: customNodeMenu[cat]
                    },
                    callback: cb
                });
            }
        }
        return menu;
    }

    LGraphCanvas.onMenuAdd = function(node, options, e, prev_menu, callback) {
        var canvas = LGraphCanvas.active_canvas;
        var graph = canvas.graph;
        if (!graph) return;

        var cb = createNodeCallback(canvas, graph, callback);
        var menu = buildMenu(cb);

        new LiteGraph.ContextMenu(menu, { event: e, parentMenu: prev_menu }, canvas.getCanvasWindow());
        return false;
    };

    // 注册自定义节点类型到 LiteGraph
    function registerCustomNodeType(def) {
        var type = def.type;
        if (LiteGraph.registered_node_types[type]) return;

        var Constructor = function() {
            if (def.inputs) {
                for (var i = 0; i < def.inputs.length; i++) {
                    var inp = def.inputs[i];
                    var t = (inp.type === -1 || inp.type === "-1") ? LiteGraph.ACTION : inp.type;
                    this.addInput(inp.name, t);
                }
            }
            if (def.outputs) {
                for (var i = 0; i < def.outputs.length; i++) {
                    var out = def.outputs[i];
                    var t = (out.type === -1 || out.type === "-1") ? LiteGraph.EVENT : out.type;
                    this.addOutput(out.name, t);
                }
            }
            this.properties = def.properties ? JSON.parse(JSON.stringify(def.properties)) : {};
            this.boxcolor = def.boxcolor || "#666";
        };
        Constructor.title = def.title || type;
        Constructor.desc = def.desc || "";
        LiteGraph.registerNodeType(type, Constructor);
        registerCustomNodeMenu(def);
    }

    // 暴露到全局，供 app.js 注册插件内嵌节点
    window.registerCustomNodeType = registerCustomNodeType;

    // 从服务器加载自定义节点
    window.loadCustomNodes = function() {
        return fetch("/api/custom-nodes")
            .then(function(r) { return r.json(); })
            .then(function(data) {
                if (data && data.code === 0 && data.data) {
                    for (var i = 0; i < data.data.length; i++) {
                        registerCustomNodeType(data.data[i]);
                    }
                }
            })
            .catch(function(err) {
                console.warn("load custom nodes failed:", err);
            });
    };
})();

// ==================== 事件节点 ====================

function OnGroupMessageNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("消息文本", "string");
    this.addOutput("发送者ID", "string");
    this.addOutput("发送者昵称", "string");
    this.addOutput("群聊ID", "string");
    this.addOutput("消息ID", "string");
    this.addOutput("所有@ID", "array");
    this.addOutput("所有@昵称", "array");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnGroupMessageNode.title = "收到群消息";
OnGroupMessageNode.desc = "当机器人在群聊中被@时触发";
LiteGraph.registerNodeType("event/on_group_message", OnGroupMessageNode);

function OnC2CMessageNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("消息文本", "string");
    this.addOutput("发送者ID", "string");
    this.addOutput("发送者昵称", "string");
    this.addOutput("消息ID", "string");
    this.addOutput("所有@ID", "array");
    this.addOutput("所有@昵称", "array");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnC2CMessageNode.title = "收到私聊消息";
OnC2CMessageNode.desc = "当收到用户私聊消息时触发";
LiteGraph.registerNodeType("event/on_c2c_message", OnC2CMessageNode);

function OnCommandNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("指令名", "string");
    this.addOutput("参数列表", "array");
    this.addOutput("参数文本", "string");
    this.addOutput("清洗后消息", "string");
    this.addOutput("原始消息", "string");
    this.addOutput("发送者ID", "string");
    this.addOutput("来源ID", "string");
    this.addOutput("消息ID", "string");
    this.addOutput("第一个@的ID", "string");
    this.addOutput("第一个@的昵称", "string");
    this.addOutput("所有@ID", "array");
    this.addOutput("所有@昵称", "array");
    this.properties = { command: "", prefix: "/" };
    this.addWidget("text", "指令名", "", { property: "command" });
    this.addWidget("text", "指令前缀", "/", { property: "prefix" });
    this.boxcolor = "#3a3";
}
OnCommandNode.title = "收到指令";
OnCommandNode.desc = "当收到以指定前缀开头的指令时触发，自动解析参数和@用户，支持 /指令 @用户 格式";
LiteGraph.registerNodeType("event/on_command", OnCommandNode);

function OnGuildMessageNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("消息文本", "string");
    this.addOutput("发送者ID", "string");
    this.addOutput("发送者昵称", "string");
    this.addOutput("频道ID", "string");
    this.addOutput("GuildID", "string");
    this.addOutput("消息ID", "string");
    this.addOutput("所有@ID", "array");
    this.addOutput("所有@昵称", "array");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnGuildMessageNode.title = "收到频道消息";
OnGuildMessageNode.desc = "当在频道中被@时触发";
LiteGraph.registerNodeType("event/on_guild_message", OnGuildMessageNode);

function OnDirectMessageNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("消息文本", "string");
    this.addOutput("发送者ID", "string");
    this.addOutput("发送者昵称", "string");
    this.addOutput("频道ID", "string");
    this.addOutput("GuildID", "string");
    this.addOutput("消息ID", "string");
    this.addOutput("所有@ID", "array");
    this.addOutput("所有@昵称", "array");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnDirectMessageNode.title = "收到频道私信";
OnDirectMessageNode.desc = "当收到频道私信消息时触发";
LiteGraph.registerNodeType("event/on_direct_message", OnDirectMessageNode);

function OnC2CFriendNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("用户OpenID", "string");
    this.addOutput("昵称", "string");
    this.addOutput("时间戳", "number");
    this.addOutput("事件数据", "object");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnC2CFriendNode.title = "好友添加/删除";
OnC2CFriendNode.desc = "当用户添加或删除机器人好友时触发";
LiteGraph.registerNodeType("event/on_c2c_friend", OnC2CFriendNode);

function OnInteractionNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("互动类型", "string");
    this.addOutput("GuildID", "string");
    this.addOutput("ChannelID", "string");
    this.addOutput("事件数据", "object");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnInteractionNode.title = "互动事件";
OnInteractionNode.desc = "当用户点击按钮、菜单等互动组件时触发";
LiteGraph.registerNodeType("event/on_interaction", OnInteractionNode);

function OnSubscribeMsgStatusNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("群OpenID", "string");
    this.addOutput("用户OpenID", "string");
    this.addOutput("授权结果", "array");
    this.addOutput("事件数据", "object");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnSubscribeMsgStatusNode.title = "订阅消息状态变更";
OnSubscribeMsgStatusNode.desc = "当订阅消息模板授权状态变更时触发";
LiteGraph.registerNodeType("event/on_subscribe_msg_status", OnSubscribeMsgStatusNode);

function OnGroupManageNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("事件类型", "string");
    this.addOutput("群OpenID", "string");
    this.addOutput("操作者OpenID", "string");
    this.addOutput("时间戳", "number");
    this.addOutput("事件数据", "object");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnGroupManageNode.title = "群管理事件";
OnGroupManageNode.desc = "机器人进退群、主动消息权限变更时触发（GROUP_ADD_ROBOT/GROUP_DEL_ROBOT/GROUP_MSG_REJECT/GROUP_MSG_RECEIVE）";
LiteGraph.registerNodeType("event/on_group_manage", OnGroupManageNode);

function OnC2CManageNode() {
    this.addOutput("触发", LiteGraph.EVENT);
    this.addOutput("事件类型", "string");
    this.addOutput("用户OpenID", "string");
    this.addOutput("时间戳", "number");
    this.addOutput("事件数据", "object");
    this.properties = {};
    this.boxcolor = "#3a3";
}
OnC2CManageNode.title = "C2C管理事件";
OnC2CManageNode.desc = "用户主动消息权限变更时触发（C2C_MSG_REJECT/C2C_MSG_RECEIVE）";
LiteGraph.registerNodeType("event/on_c2c_manage", OnC2CManageNode);

// ==================== 消息处理节点 ====================

function ParseCommandNode() {
    this.addInput("消息文本", "string");
    this.addInput("指令前缀", "string");
    this.addOutput("指令名", "string");
    this.addOutput("参数列表", "array");
    this.addOutput("参数文本", "string");
    this.addOutput("清洗后文本", "string");
    this.properties = { prefix: "/" };
    this.addWidget("text", "默认前缀", "/", { property: "prefix" });
    this.boxcolor = "#369";
}
ParseCommandNode.title = "解析指令参数";
ParseCommandNode.desc = "从消息文本中解析指令名和参数，自动清洗@格式，支持多处连接复用";
LiteGraph.registerNodeType("message/parse_command", ParseCommandNode);

function ExtractMentionUserNode() {
    this.addInput("消息文本", "string");
    this.addOutput("第一个@的昵称", "string");
    this.addOutput("所有@昵称", "array");
    this.addOutput("第一个@的ID", "string");
    this.addOutput("所有@ID", "array");
    this.boxcolor = "#369";
}
ExtractMentionUserNode.title = "提取@用户昵称";
ExtractMentionUserNode.desc = "从消息中提取被@用户的昵称和ID列表";
ExtractMentionUserNode.prototype.onExecute = function() {
    var text = this.getInputData(0) || "";
    var mentions = [];
    var regex = /<@(.*?)>/g;
    var match;
    while ((match = regex.exec(text)) !== null) {
        mentions.push(match[1]);
    }
    this.setOutputData(0, mentions.length > 0 ? mentions[0] : "");
    this.setOutputData(1, mentions);
};
LiteGraph.registerNodeType("message/extract_mention_user", ExtractMentionUserNode);

function ParseImagesNode() {
    this.addOutput("图片URL列表", "array");
    this.addOutput("图片数量", "number");
    this.addOutput("第一张图片URL", "string");
    this.addOutput("图片文件名列表", "array");
    this.addOutput("视频URL列表", "array");
    this.addOutput("语音URL列表", "array");
    this.boxcolor = "#369";
}
ParseImagesNode.title = "提取消息中的图片";
ParseImagesNode.desc = "从消息附件中提取图片、视频、语音等媒体资源URL";
LiteGraph.registerNodeType("message/parse_images", ParseImagesNode);

// ==================== 发送消息节点 ====================

function SendMessageNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("回复内容", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { template: "" };
    this.addWidget("text", "回复内容", "", { property: "template", multiline: true });
    this.boxcolor = "#33a";
}
SendMessageNode.title = "回复消息";
SendMessageNode.desc = "回复当前消息来源（群聊回复群，私聊回复私聊）";
LiteGraph.registerNodeType("action/send_message", SendMessageNode);

function SendGroupMessageNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("消息内容", "string");
    this.addInput("群聊ID", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { template: "" };
    this.addWidget("text", "消息模板", "", { property: "template", multiline: true });
    this.boxcolor = "#33a";
}
SendGroupMessageNode.title = "发送群消息";
SendGroupMessageNode.desc = "向指定群聊发送消息，群聊ID留空则回复来源群";
LiteGraph.registerNodeType("action/send_group_message", SendGroupMessageNode);

function SendC2CMessageNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("消息内容", "string");
    this.addInput("用户ID", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { template: "" };
    this.addWidget("text", "消息模板", "", { property: "template", multiline: true });
    this.boxcolor = "#33a";
}
SendC2CMessageNode.title = "发送私聊消息";
SendC2CMessageNode.desc = "向指定用户发送私聊消息，用户ID留空则回复来源用户";
LiteGraph.registerNodeType("action/send_c2c_message", SendC2CMessageNode);

function SendImageNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("图片URL", "string");
    this.addInput("文本(备用)", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = {};
    this.boxcolor = "#33a";
}
SendImageNode.title = "发送图片";
SendImageNode.desc = "发送图片消息，图片URL为空时发送备用文本";
LiteGraph.registerNodeType("action/send_image", SendImageNode);

function SendRichMediaNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("文本内容", "string");
    this.addInput("图片URL", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { text: "", image_url: "" };
    this.boxcolor = "#33a";
}
SendRichMediaNode.title = "发送图文消息";
SendRichMediaNode.desc = "发送图文混合消息，同时发送文本和图片";
LiteGraph.registerNodeType("action/send_rich_media", SendRichMediaNode);

function RecallMessageNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("消息ID", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = {};
    this.boxcolor = "#33a";
}
RecallMessageNode.title = "撤回消息";
RecallMessageNode.desc = "撤回指定消息，消息ID留空则撤回当前触发消息";
LiteGraph.registerNodeType("action/recall_message", RecallMessageNode);

// ==================== Markdown / 按钮 / 视频发送节点 ====================

function SendMarkdownNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("Markdown内容", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { content: "" };
    this.addWidget("text", "Markdown内容", "", { property: "content", multiline: true });
    this.boxcolor = "#33a";
}
SendMarkdownNode.title = "发送Markdown";
SendMarkdownNode.desc = "发送Markdown格式消息（msg_type=2）";
LiteGraph.registerNodeType("action/send_markdown", SendMarkdownNode);

function SendButtonsNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("Markdown内容", "string");
    this.addInput("按钮配置", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { markdown: "", buttons: "" };
    this.addWidget("text", "Markdown内容(可选)", "", { property: "markdown", multiline: true });
    this.addWidget("text", "按钮配置JSON", "", { property: "buttons" });
    this.boxcolor = "#33a";
}
SendButtonsNode.title = "发送按钮消息";
SendButtonsNode.desc = "发送带内嵌键盘按钮的消息（Markdown可空）";
LiteGraph.registerNodeType("action/send_buttons", SendButtonsNode);

function SendMarkdownWithButtonsNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("Markdown内容", "string");
    this.addInput("按钮配置", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { markdown: "", buttons: "" };
    this.addWidget("text", "Markdown内容", "", { property: "markdown", multiline: true });
    this.addWidget("text", "按钮配置JSON", "", { property: "buttons" });
    this.boxcolor = "#33a";
}
SendMarkdownWithButtonsNode.title = "Markdown+按钮";
SendMarkdownWithButtonsNode.desc = "同时发送Markdown内容和按钮（联动）";
LiteGraph.registerNodeType("action/send_markdown_with_buttons", SendMarkdownWithButtonsNode);

function SendVideoNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("视频URL", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { video_url: "" };
    this.addWidget("text", "视频URL", "", { property: "video_url" });
    this.boxcolor = "#33a";
}
SendVideoNode.title = "发送视频";
SendVideoNode.desc = "发送视频消息（msg_type=7，FileType=2）";
LiteGraph.registerNodeType("action/send_video", SendVideoNode);

// ==================== HTML 渲染节点 ====================

function HTMLToImageNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("HTML内容", "string");
    this.addInput("URL", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.addOutput("图片URL", "string");
    this.properties = { html: "", url: "", width: 800 };
    this.addWidget("text", "HTML内容", "", { property: "html", multiline: true });
    this.addWidget("text", "URL", "", { property: "url" });
    this.addWidget("number", "宽度", 800, { property: "width" });
    this.boxcolor = "#a73";
}
HTMLToImageNode.title = "HTML转图片";
HTMLToImageNode.desc = "使用无头浏览器(chromedp)将HTML字符串或URL渲染为PNG图片";
LiteGraph.registerNodeType("render/html_to_image", HTMLToImageNode);

function HTMLTemplateNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("标题", "string");
    this.addInput("正文", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.addOutput("HTML内容", "string");
    this.properties = {
        template_name: "card",
        title: "",
        body: "",
        font_size: 16,
        font_color: "#333333",
        bg_color: "#ffffff"
    };
    this.addWidget("combo", "模板", "card", { property: "template_name", values: ["card", "article", "notice", "custom"] });
    this.addWidget("text", "标题", "", { property: "title", multiline: true });
    this.addWidget("text", "正文", "", { property: "body", multiline: true });
    this.addWidget("number", "字号", 16, { property: "font_size" });
    this.addWidget("text", "字体颜色", "#333333", { property: "font_color" });
    this.addWidget("text", "背景颜色", "#ffffff", { property: "bg_color" });
    this.boxcolor = "#a73";
}
HTMLTemplateNode.title = "HTML模板";
HTMLTemplateNode.desc = "使用内置模板(card/article/notice)拼接HTML，可联动HTML转图片节点";
LiteGraph.registerNodeType("render/html_template", HTMLTemplateNode);

// ==================== 文件操作节点 ====================

function FileReadNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("文件路径", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.addOutput("内容", "string");
    this.addOutput("成功", "boolean");
    this.properties = { path: "" };
    this.addWidget("text", "文件路径", "", { property: "path" });
    this.boxcolor = "#666";
}
FileReadNode.title = "读取文件";
FileReadNode.desc = "读取本地文件内容（文本）";
LiteGraph.registerNodeType("file/read", FileReadNode);

function FileWriteNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("文件路径", "string");
    this.addInput("内容", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.addOutput("成功", "boolean");
    this.properties = { path: "", content: "", append: false };
    this.addWidget("text", "文件路径", "", { property: "path" });
    this.addWidget("text", "内容", "", { property: "content", multiline: true });
    this.addWidget("toggle", "追加", false, { property: "append" });
    this.boxcolor = "#666";
}
FileWriteNode.title = "写入文件";
FileWriteNode.desc = "写入本地文件（可选追加或覆盖）";
LiteGraph.registerNodeType("file/write", FileWriteNode);

// ==================== 条件判断节点 ====================

function TextContainsNode() {
    this.addInput("文本", "string");
    this.addOutput("结果", "boolean");
    this.addOutput("包含", LiteGraph.EVENT);
    this.addOutput("不包含", LiteGraph.EVENT);
    this.properties = { keyword: "" };
    this.addWidget("text", "关键词", "", { property: "keyword" });
    this.boxcolor = "#aa3";
}
TextContainsNode.title = "文本包含";
TextContainsNode.desc = "判断文本中是否包含指定关键词";
TextContainsNode.prototype.onExecute = function() {
    var text = this.getInputData(0) || "";
    var keyword = this.properties.keyword || "";
    var result = keyword && text.includes(keyword);
    this.setOutputData(0, result);
    if (result) this.triggerSlot(1);
    else this.triggerSlot(2);
};
LiteGraph.registerNodeType("condition/text_contains", TextContainsNode);

function TextEqualsNode() {
    this.addInput("文本", "string");
    this.addOutput("结果", "boolean");
    this.addOutput("相等", LiteGraph.EVENT);
    this.addOutput("不等", LiteGraph.EVENT);
    this.properties = { expected: "" };
    this.addWidget("text", "期望值", "", { property: "expected" });
    this.boxcolor = "#aa3";
}
TextEqualsNode.title = "文本相等";
TextEqualsNode.desc = "判断文本是否完全等于期望值";
TextEqualsNode.prototype.onExecute = function() {
    var text = this.getInputData(0) || "";
    var expected = this.properties.expected || "";
    var result = text === expected;
    this.setOutputData(0, result);
    if (result) this.triggerSlot(1);
    else this.triggerSlot(2);
};
LiteGraph.registerNodeType("condition/text_equals", TextEqualsNode);

function TextMatchesNode() {
    this.addInput("文本", "string");
    this.addOutput("结果", "boolean");
    this.addOutput("匹配", LiteGraph.EVENT);
    this.addOutput("不匹配", LiteGraph.EVENT);
    this.addOutput("匹配结果", "array");
    this.properties = { pattern: "" };
    this.addWidget("text", "正则表达式", "", { property: "pattern" });
    this.boxcolor = "#aa3";
}
TextMatchesNode.title = "正则匹配";
TextMatchesNode.desc = "使用正则表达式匹配文本，可提取捕获组";
TextMatchesNode.prototype.onExecute = function() {
    var text = this.getInputData(0) || "";
    var pattern = this.properties.pattern || "";
    var result = false;
    var matches = [];
    try {
        if (pattern) {
            var re = new RegExp(pattern);
            var m = re.exec(text);
            if (m) { result = true; matches = Array.from(m); }
        }
    } catch (e) {}
    this.setOutputData(0, result);
    this.setOutputData(3, matches);
    if (result) this.triggerSlot(1);
    else this.triggerSlot(2);
};
LiteGraph.registerNodeType("condition/text_matches", TextMatchesNode);

function NumberCompareNode() {
    this.addInput("数值A", "number");
    this.addInput("数值B", "number");
    this.addOutput("结果", "boolean");
    this.addOutput("成立", LiteGraph.EVENT);
    this.addOutput("不成立", LiteGraph.EVENT);
    this.properties = { operator: "==" };
    this.addWidget("combo", "比较方式", "==", { property: "operator", values: ["==", "!=", ">", ">=", "<", "<="] });
    this.boxcolor = "#aa3";
}
NumberCompareNode.title = "数字比较";
NumberCompareNode.desc = "比较两个数字的大小关系";
NumberCompareNode.prototype.onExecute = function() {
    var a = parseFloat(this.getInputData(0)) || 0;
    var b = parseFloat(this.getInputData(1)) || 0;
    var op = this.properties.operator || "==";
    var result = false;
    switch (op) {
        case "==": result = a == b; break;
        case "!=": result = a != b; break;
        case ">": result = a > b; break;
        case ">=": result = a >= b; break;
        case "<": result = a < b; break;
        case "<=": result = a <= b; break;
    }
    this.setOutputData(0, result);
    if (result) this.triggerSlot(1);
    else this.triggerSlot(2);
};
LiteGraph.registerNodeType("condition/number_compare", NumberCompareNode);

function IsEmptyNode() {
    this.addInput("值", 0);
    this.addOutput("结果", "boolean");
    this.addOutput("值", 0);
    this.addOutput("为空", LiteGraph.EVENT);
    this.addOutput("不为空", LiteGraph.EVENT);
    this.boxcolor = "#aa3";
}
IsEmptyNode.title = "是否为空";
IsEmptyNode.desc = "判断值是否为空（null、空字符串、空数组）";
IsEmptyNode.prototype.onExecute = function() {
    var val = this.getInputData(0);
    var result = val === null || val === undefined || val === "" || (Array.isArray(val) && val.length === 0);
    this.setOutputData(0, result);
    this.setOutputData(1, val);
    if (result) this.triggerSlot(3);
    else this.triggerSlot(2);
};
LiteGraph.registerNodeType("condition/is_empty", IsEmptyNode);

function BranchNode() {
    this.addInput("条件", "boolean");
    this.addOutput("条件成立", LiteGraph.EVENT);
    this.addOutput("条件不成立", LiteGraph.EVENT);
    this.boxcolor = "#aa3";
}
BranchNode.title = "条件分支";
BranchNode.desc = "根据条件值选择执行路径";
BranchNode.prototype.onExecute = function() {
    if (this.getInputData(0)) this.triggerSlot(0);
    else this.triggerSlot(1);
};
LiteGraph.registerNodeType("logic/branch", BranchNode);

function AndNode() {
    this.addInput("条件A", "boolean");
    this.addInput("条件B", "boolean");
    this.addOutput("结果", "boolean");
    this.boxcolor = "#aa3";
}
AndNode.title = "逻辑与";
AndNode.desc = "两个条件都成立时结果为真";
AndNode.prototype.onExecute = function() {
    this.setOutputData(0, !!(this.getInputData(0) && this.getInputData(1)));
};
LiteGraph.registerNodeType("logic/and", AndNode);

function OrNode() {
    this.addInput("条件A", "boolean");
    this.addInput("条件B", "boolean");
    this.addOutput("结果", "boolean");
    this.boxcolor = "#aa3";
}
OrNode.title = "逻辑或";
OrNode.desc = "任一条件成立时结果为真";
OrNode.prototype.onExecute = function() {
    this.setOutputData(0, !!(this.getInputData(0) || this.getInputData(1)));
};
LiteGraph.registerNodeType("logic/or", OrNode);

function NotNode() {
    this.addInput("条件", "boolean");
    this.addOutput("结果", "boolean");
    this.boxcolor = "#aa3";
}
NotNode.title = "逻辑非";
NotNode.desc = "取反条件结果";
NotNode.prototype.onExecute = function() {
    this.setOutputData(0, !this.getInputData(0));
};
LiteGraph.registerNodeType("logic/not", NotNode);

// ==================== 文本处理节点 ====================

function TemplateNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("结果", "string");
    this.properties = { template: "" };
    this.addWidget("text", "模板内容", "", { property: "template", multiline: true });
    this.boxcolor = "#a3a";
}
TemplateNode.title = "文本模板";
TemplateNode.desc = "生成固定文本，支持 {变量名} 占位符替换";
TemplateNode.prototype.onExecute = function() {
    this.setOutputData(0, this.properties.template || "");
};
LiteGraph.registerNodeType("text/template", TemplateNode);

function ConcatNode() {
    this.addInput("文本A", "string");
    this.addInput("文本B", "string");
    this.addOutput("结果", "string");
    this.properties = { separator: "" };
    this.addWidget("text", "分隔符", "", { property: "separator" });
    this.boxcolor = "#a3a";
}
ConcatNode.title = "文本拼接";
ConcatNode.desc = "将两段文本拼接在一起，可设置分隔符";
ConcatNode.prototype.onExecute = function() {
    var a = this.getInputData(0) || "";
    var b = this.getInputData(1) || "";
    this.setOutputData(0, a + this.properties.separator + b);
};
LiteGraph.registerNodeType("text/concat", ConcatNode);

function SubstringNode() {
    this.addInput("文本", "string");
    this.addOutput("结果", "string");
    this.properties = { start: 0, length: 10 };
    this.addWidget("number", "起始位置", 0, { property: "start" });
    this.addWidget("number", "截取长度", 10, { property: "length" });
    this.boxcolor = "#a3a";
}
SubstringNode.title = "文本截取";
SubstringNode.desc = "截取文本的一部分";
SubstringNode.prototype.onExecute = function() {
    var text = this.getInputData(0) || "";
    this.setOutputData(0, text.substr(this.properties.start || 0, this.properties.length));
};
LiteGraph.registerNodeType("text/substring", SubstringNode);

function ReplaceNode() {
    this.addInput("文本", "string");
    this.addOutput("结果", "string");
    this.properties = { search: "", replace: "" };
    this.addWidget("text", "查找内容", "", { property: "search" });
    this.addWidget("text", "替换为", "", { property: "replace" });
    this.boxcolor = "#a3a";
}
ReplaceNode.title = "文本替换";
ReplaceNode.desc = "将文本中的指定内容替换为新内容";
ReplaceNode.prototype.onExecute = function() {
    var text = this.getInputData(0) || "";
    var search = this.properties.search || "";
    if (search) text = text.split(search).join(this.properties.replace || "");
    this.setOutputData(0, text);
};
LiteGraph.registerNodeType("text/replace", ReplaceNode);

function SplitNode() {
    this.addInput("文本", "string");
    this.addOutput("数组", "array");
    this.addOutput("长度", "number");
    this.properties = { separator: "," };
    this.addWidget("text", "分隔符", ",", { property: "separator" });
    this.boxcolor = "#a3a";
}
SplitNode.title = "文本分割";
SplitNode.desc = "按分隔符将文本拆分为数组";
SplitNode.prototype.onExecute = function() {
    var text = this.getInputData(0) || "";
    var arr = text.split(this.properties.separator || ",");
    this.setOutputData(0, arr);
    this.setOutputData(1, arr.length);
};
LiteGraph.registerNodeType("text/split", SplitNode);

function TextLengthNode() {
    this.addInput("文本", "string");
    this.addOutput("长度", "number");
    this.boxcolor = "#a3a";
}
TextLengthNode.title = "文本长度";
TextLengthNode.desc = "获取文本的字符数";
TextLengthNode.prototype.onExecute = function() {
    this.setOutputData(0, (this.getInputData(0) || "").length);
};
LiteGraph.registerNodeType("text/length", TextLengthNode);

function ToUpperNode() {
    this.addInput("文本", "string");
    this.addOutput("结果", "string");
    this.boxcolor = "#a3a";
}
ToUpperNode.title = "转大写";
ToUpperNode.desc = "将文本转换为大写";
ToUpperNode.prototype.onExecute = function() {
    this.setOutputData(0, (this.getInputData(0) || "").toUpperCase());
};
LiteGraph.registerNodeType("text/to_upper", ToUpperNode);

function ToLowerNode() {
    this.addInput("文本", "string");
    this.addOutput("结果", "string");
    this.boxcolor = "#a3a";
}
ToLowerNode.title = "转小写";
ToLowerNode.desc = "将文本转换为小写";
ToLowerNode.prototype.onExecute = function() {
    this.setOutputData(0, (this.getInputData(0) || "").toLowerCase());
};
LiteGraph.registerNodeType("text/to_lower", ToLowerNode);

function TrimNode() {
    this.addInput("文本", "string"); this.addOutput("结果", "string"); this.boxcolor = "#a3a";
}
TrimNode.title = "去空格"; TrimNode.desc = "去除文本首尾空格";
TrimNode.prototype.onExecute = function() { this.setOutputData(0, (this.getInputData(0) || "").trim()); };
LiteGraph.registerNodeType("text/trim", TrimNode);

function FindNode() {
    this.addInput("文本", "string"); this.addInput("查找内容", "string");
    this.addOutput("位置", "number"); this.boxcolor = "#a3a";
}
FindNode.title = "查找文本"; FindNode.desc = "查找文本中第一次出现的位置，未找到返回-1";
FindNode.prototype.onExecute = function() { var t=this.getInputData(0)||"", s=this.getInputData(1)||""; this.setOutputData(0, s?t.indexOf(s):-1); };
LiteGraph.registerNodeType("text/find", FindNode);

function CharAtNode() {
    this.addInput("文本", "string"); this.addInput("索引", "number");
    this.addOutput("字符", "string"); this.boxcolor = "#a3a";
}
CharAtNode.title = "获取字符"; CharAtNode.desc = "获取文本中指定位置的字符";
CharAtNode.prototype.onExecute = function() { var t=this.getInputData(0)||"", i=parseInt(this.getInputData(1))||0; this.setOutputData(0, i>=0&&i<t.length?t.charAt(i):""); };
LiteGraph.registerNodeType("text/char_at", CharAtNode);

function RegexReplaceNode() {
    this.addInput("文本", "string");
    this.addOutput("结果", "string");
    this.properties = { pattern: "", replace: "", global: true };
    this.addWidget("text", "正则表达式", "", { property: "pattern" });
    this.addWidget("text", "替换为", "", { property: "replace" });
    this.addWidget("toggle", "全局替换", true, { property: "global" });
    this.boxcolor = "#a3a";
}
RegexReplaceNode.title = "正则替换"; RegexReplaceNode.desc = "使用正则表达式替换文本内容";
RegexReplaceNode.prototype.onExecute = function() { var t=this.getInputData(0)||""; try{if(this.properties.pattern){var re=new RegExp(this.properties.pattern, this.properties.global?"g":""); t=t.replace(re, this.properties.replace||"");}}catch(e){} this.setOutputData(0, t); };
LiteGraph.registerNodeType("text/regex_replace", RegexReplaceNode);

// ==================== 数学运算节点 ====================

function AddNode() {
    this.addInput("A", "number"); this.addInput("B", "number");
    this.addOutput("A+B", "number"); this.boxcolor = "#399";
}
AddNode.title = "加法"; AddNode.desc = "计算 A + B";
AddNode.prototype.onExecute = function() {
    this.setOutputData(0, (parseFloat(this.getInputData(0))||0) + (parseFloat(this.getInputData(1))||0));
};
LiteGraph.registerNodeType("math/add", AddNode);

function SubtractNode() {
    this.addInput("A", "number"); this.addInput("B", "number");
    this.addOutput("A-B", "number"); this.boxcolor = "#399";
}
SubtractNode.title = "减法"; SubtractNode.desc = "计算 A - B";
SubtractNode.prototype.onExecute = function() {
    this.setOutputData(0, (parseFloat(this.getInputData(0))||0) - (parseFloat(this.getInputData(1))||0));
};
LiteGraph.registerNodeType("math/subtract", SubtractNode);

function MultiplyNode() {
    this.addInput("A", "number"); this.addInput("B", "number");
    this.addOutput("A*B", "number"); this.boxcolor = "#399";
}
MultiplyNode.title = "乘法"; MultiplyNode.desc = "计算 A * B";
MultiplyNode.prototype.onExecute = function() {
    this.setOutputData(0, (parseFloat(this.getInputData(0))||0) * (parseFloat(this.getInputData(1))||0));
};
LiteGraph.registerNodeType("math/multiply", MultiplyNode);

function DivideNode() {
    this.addInput("A", "number"); this.addInput("B", "number");
    this.addOutput("A/B", "number"); this.boxcolor = "#399";
}
DivideNode.title = "除法"; DivideNode.desc = "计算 A / B，B为0时返回0";
DivideNode.prototype.onExecute = function() {
    var b = parseFloat(this.getInputData(1)) || 0;
    this.setOutputData(0, b !== 0 ? (parseFloat(this.getInputData(0))||0) / b : 0);
};
LiteGraph.registerNodeType("math/divide", DivideNode);

function ModuloNode() {
    this.addInput("A", "number"); this.addInput("B", "number");
    this.addOutput("A%B", "number"); this.boxcolor = "#399";
}
ModuloNode.title = "取模"; ModuloNode.desc = "计算 A % B（取余数）";
ModuloNode.prototype.onExecute = function() {
    var b = parseFloat(this.getInputData(1)) || 0;
    this.setOutputData(0, b !== 0 ? (parseFloat(this.getInputData(0))||0) % b : 0);
};
LiteGraph.registerNodeType("math/modulo", ModuloNode);

function RandomNode() {
    this.addOutput("随机数", "number");
    this.properties = { min: 0, max: 100 };
    this.addWidget("number", "最小值", 0, { property: "min" });
    this.addWidget("number", "最大值", 100, { property: "max" });
    this.boxcolor = "#399";
}
RandomNode.title = "随机数"; RandomNode.desc = "生成指定范围内的随机整数";
RandomNode.prototype.onExecute = function() {
    var min = this.properties.min || 0;
    var max = this.properties.max || 100;
    this.setOutputData(0, Math.floor(Math.random() * (max - min + 1)) + min);
};
LiteGraph.registerNodeType("math/random", RandomNode);

function RoundNode() {
    this.addInput("数值", "number"); this.addOutput("结果", "number"); this.boxcolor = "#399";
}
RoundNode.title = "四舍五入"; RoundNode.desc = "对小数进行四舍五入";
RoundNode.prototype.onExecute = function() { this.setOutputData(0, Math.round(parseFloat(this.getInputData(0))||0)); };
LiteGraph.registerNodeType("math/round", RoundNode);

function FloorNode() {
    this.addInput("数值", "number"); this.addOutput("结果", "number"); this.boxcolor = "#399";
}
FloorNode.title = "向下取整"; FloorNode.desc = "舍去小数部分";
FloorNode.prototype.onExecute = function() { this.setOutputData(0, Math.floor(parseFloat(this.getInputData(0))||0)); };
LiteGraph.registerNodeType("math/floor", FloorNode);

function CeilNode() {
    this.addInput("数值", "number"); this.addOutput("结果", "number"); this.boxcolor = "#399";
}
CeilNode.title = "向上取整"; CeilNode.desc = "向上进位取整";
CeilNode.prototype.onExecute = function() { this.setOutputData(0, Math.ceil(parseFloat(this.getInputData(0))||0)); };
LiteGraph.registerNodeType("math/ceil", CeilNode);

function AbsNode() {
    this.addInput("数值", "number"); this.addOutput("结果", "number"); this.boxcolor = "#399";
}
AbsNode.title = "绝对值"; AbsNode.desc = "取数值的绝对值";
AbsNode.prototype.onExecute = function() { this.setOutputData(0, Math.abs(parseFloat(this.getInputData(0))||0)); };
LiteGraph.registerNodeType("math/abs", AbsNode);

function TypeConvertNode() {
    this.addInput("值", 0);
    this.addOutput("结果", 0);
    this.properties = { target_type: "string" };
    this.addWidget("combo", "目标类型", "string", { property: "target_type", values: ["string", "number", "boolean"] });
    this.boxcolor = "#399";
}
TypeConvertNode.title = "类型转换"; TypeConvertNode.desc = "将值转换为目标类型";
TypeConvertNode.prototype.onExecute = function() {
    var val = this.getInputData(0);
    var type = this.properties.target_type;
    if (type === "number") {
        this.setOutputData(0, parseFloat(val) || 0);
    } else if (type === "boolean") {
        this.setOutputData(0, !!val && val !== "false" && val !== "0");
    } else {
        this.setOutputData(0, val !== undefined && val !== null ? String(val) : "");
    }
};
LiteGraph.registerNodeType("math/type_convert", TypeConvertNode);

function ToNumberNode() {
    this.addInput("文本", "string"); this.addOutput("结果", "number"); this.boxcolor = "#399";
}
ToNumberNode.title = "转换为数字"; ToNumberNode.desc = "将文本转换为数字";
ToNumberNode.prototype.onExecute = function() { this.setOutputData(0, parseFloat(this.getInputData(0))||0); };
LiteGraph.registerNodeType("math/to_number", ToNumberNode);

function SqrtNode() {
    this.addInput("数值", "number"); this.addOutput("结果", "number"); this.boxcolor = "#399";
}
SqrtNode.title = "平方根"; SqrtNode.desc = "计算数值的平方根";
SqrtNode.prototype.onExecute = function() { this.setOutputData(0, Math.sqrt(Math.max(0, parseFloat(this.getInputData(0))||0))); };
LiteGraph.registerNodeType("math/sqrt", SqrtNode);

function SinNode() {
    this.addInput("角度", "number"); this.addOutput("结果", "number"); this.boxcolor = "#399";
}
SinNode.title = "正弦"; SinNode.desc = "计算角度的正弦值（角度制）";
SinNode.prototype.onExecute = function() { this.setOutputData(0, Math.sin(parseFloat(this.getInputData(0))||0 * Math.PI / 180)); };
LiteGraph.registerNodeType("math/sin", SinNode);

function NumberNode() {
    this.addOutput("数值", "number"); this.boxcolor = "#399";
    this.properties = { value: 0 };
    this.addWidget("number", "值", 0, { property: "value", step: 1, precision: 6 });
}
NumberNode.title = "数字常量"; NumberNode.desc = "输出一个固定数值";
NumberNode.prototype.onExecute = function() {
    var v = parseFloat(this.properties.value);
    this.setOutputData(0, isNaN(v) ? 0 : v);
};
LiteGraph.registerNodeType("math/number", NumberNode);

function PiNode() {
    this.addOutput("π", "number"); this.boxcolor = "#399";
}
PiNode.title = "π常量"; PiNode.desc = "输出圆周率π的值";
PiNode.prototype.onExecute = function() { this.setOutputData(0, Math.PI); };
LiteGraph.registerNodeType("math/pi", PiNode);

function IsEvenNode() {
    this.addInput("数值", "number"); this.addOutput("结果", "boolean"); this.boxcolor = "#399";
}
IsEvenNode.title = "判断偶数"; IsEvenNode.desc = "判断数值是否为偶数";
IsEvenNode.prototype.onExecute = function() { this.setOutputData(0, (parseInt(this.getInputData(0))||0) % 2 === 0); };
LiteGraph.registerNodeType("math/is_even", IsEvenNode);

function SumNode() {
    this.addInput("列表", "array"); this.addOutput("总和", "number"); this.boxcolor = "#399";
}
SumNode.title = "列表求和"; SumNode.desc = "计算列表中所有数值的和";
SumNode.prototype.onExecute = function() { var arr = this.getInputData(0)||[]; var sum = 0; for(var i=0;i<arr.length;i++) sum += parseFloat(arr[i])||0; this.setOutputData(0, sum); };
LiteGraph.registerNodeType("math/sum", SumNode);

function ClampNode() {
    this.addInput("数值", "number"); this.addInput("最小值", "number"); this.addInput("最大值", "number");
    this.addOutput("结果", "number"); this.boxcolor = "#399";
}
ClampNode.title = "限制范围"; ClampNode.desc = "将数值限制在指定范围内";
ClampNode.prototype.onExecute = function() { var v=parseFloat(this.getInputData(0))||0, min=parseFloat(this.getInputData(1))||0, max=parseFloat(this.getInputData(2))||100; this.setOutputData(0, Math.max(min, Math.min(max, v))); };
LiteGraph.registerNodeType("math/clamp", ClampNode);

function RandomFloatNode() {
    this.addOutput("随机数", "number");
    this.properties = { min: 0, max: 1 };
    this.addWidget("number", "最小值", 0, { property: "min" });
    this.addWidget("number", "最大值", 1, { property: "max" });
    this.boxcolor = "#399";
}
RandomFloatNode.title = "随机小数"; RandomFloatNode.desc = "生成指定范围内的随机小数";
RandomFloatNode.prototype.onExecute = function() { var min=this.properties.min||0, max=this.properties.max||1; this.setOutputData(0, min + Math.random()*(max-min)); };
LiteGraph.registerNodeType("math/random_float", RandomFloatNode);

// ==================== 网络请求节点 ====================

function HTTPRequestNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("URL", "string");
    this.addInput("请求体", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.addOutput("响应内容", "string");
    this.addOutput("状态码", "number");
    this.properties = { method: "GET" };
    this.addWidget("combo", "请求方法", "GET", { property: "method", values: ["GET", "POST", "PUT", "DELETE"] });
    this.boxcolor = "#a73";
}
HTTPRequestNode.title = "HTTP请求";
HTTPRequestNode.desc = "发送HTTP请求并获取响应";
HTTPRequestNode.prototype.onExecute = function() {
    this.setOutputData(1, "");
    this.setOutputData(2, 0);
};
LiteGraph.registerNodeType("http/request", HTTPRequestNode);

function JSONParseNode() {
    this.addInput("JSON文本", "string");
    this.addOutput("解析结果", "object");
    this.addOutput("字段值", 0);
    this.properties = { field: "" };
    this.addWidget("text", "字段路径", "", { property: "field" });
    this.boxcolor = "#a73";
}
JSONParseNode.title = "JSON解析";
JSONParseNode.desc = "解析JSON文本，可按路径提取字段值（如 data.name）";
JSONParseNode.prototype.onExecute = function() {
    var text = this.getInputData(0) || "";
    try {
        var obj = JSON.parse(text);
        this.setOutputData(0, obj);
        if (this.properties.field) {
            var val = obj;
            var parts = this.properties.field.split(".");
            for (var i = 0; i < parts.length && val; i++) val = val[parts[i]];
            this.setOutputData(1, val);
        }
    } catch (e) { this.setOutputData(0, null); }
};
LiteGraph.registerNodeType("json/parse", JSONParseNode);

function JSONGenerateNode() {
    this.addInput("对象", "object");
    this.addOutput("JSON文本", "string");
    this.properties = { indent: false };
    this.addWidget("toggle", "格式化输出", false, { property: "indent" });
    this.boxcolor = "#a73";
}
JSONGenerateNode.title = "JSON生成";
JSONGenerateNode.desc = "将对象转换为JSON文本";
JSONGenerateNode.prototype.onExecute = function() {
    try {
        this.setOutputData(0, this.properties.indent ? JSON.stringify(this.getInputData(0)||{}, null, 2) : JSON.stringify(this.getInputData(0)||{}));
    } catch (e) { this.setOutputData(0, ""); }
};
LiteGraph.registerNodeType("json/generate", JSONGenerateNode);

function URLEncodeNode() {
    this.addInput("文本", "string"); this.addOutput("结果", "string"); this.boxcolor = "#a73";
}
URLEncodeNode.title = "URL编码"; URLEncodeNode.desc = "对文本进行URL编码";
URLEncodeNode.prototype.onExecute = function() { this.setOutputData(0, encodeURIComponent(this.getInputData(0)||"")); };
LiteGraph.registerNodeType("http/url_encode", URLEncodeNode);

function URLDecodeNode() {
    this.addInput("文本", "string"); this.addOutput("结果", "string"); this.boxcolor = "#a73";
}
URLDecodeNode.title = "URL解码"; URLDecodeNode.desc = "对URL编码的文本进行解码";
URLDecodeNode.prototype.onExecute = function() {
    try { this.setOutputData(0, decodeURIComponent(this.getInputData(0)||"")); } catch(e) { this.setOutputData(0, this.getInputData(0)||""); }
};
LiteGraph.registerNodeType("http/url_decode", URLDecodeNode);

// ==================== 变量节点 ====================

function SetVariableNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("值", 0);
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { name: "var1" };
    this.addWidget("text", "变量名", "var1", { property: "name" });
    this.boxcolor = "#666";
}
SetVariableNode.title = "设置变量";
SetVariableNode.desc = "保存一个值到变量中，后续可通过变量名获取";
LiteGraph.registerNodeType("variable/set", SetVariableNode);

function GetVariableNode() {
    this.addOutput("值", 0);
    this.properties = { name: "var1" };
    this.addWidget("text", "变量名", "var1", { property: "name" });
    this.boxcolor = "#666";
}
GetVariableNode.title = "获取变量";
GetVariableNode.desc = "读取之前保存的变量值";
LiteGraph.registerNodeType("variable/get", GetVariableNode);

function DeleteVariableNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { name: "var1" };
    this.addWidget("text", "变量名", "var1", { property: "name" });
    this.boxcolor = "#666";
}
DeleteVariableNode.title = "删除变量";
DeleteVariableNode.desc = "删除指定变量";
LiteGraph.registerNodeType("variable/delete", DeleteVariableNode);

function ConstantNode() {
    this.addOutput("值", 0);
    this.properties = { value: "", type: "string" };
    this.addWidget("text", "值", "", { property: "value" });
    this.addWidget("combo", "类型", "string", { property: "type", values: ["string", "number", "boolean"] });
    this.boxcolor = "#666";
}
ConstantNode.title = "常量";
ConstantNode.desc = "输出一个固定的常量值";
ConstantNode.prototype.onExecute = function() {
    var val = this.properties.value;
    if (this.properties.type === "number") val = parseFloat(val) || 0;
    else if (this.properties.type === "boolean") val = val === "true";
    this.setOutputData(0, val);
};
LiteGraph.registerNodeType("variable/constant", ConstantNode);

// ==================== 时间节点 ====================

function TimeNowNode() {
    this.addOutput("时间戳(ms)", "number");
    this.addOutput("日期时间", "string");
    this.boxcolor = "#936";
}
TimeNowNode.title = "当前时间";
TimeNowNode.desc = "获取当前时间戳和格式化日期时间";
TimeNowNode.prototype.onExecute = function() {
    var now = Date.now();
    this.setOutputData(0, now);
    this.setOutputData(1, new Date(now).toLocaleString());
};
LiteGraph.registerNodeType("time/now", TimeNowNode);

function TimeFormatNode() {
    this.addInput("时间戳(ms)", "number");
    this.addOutput("格式化结果", "string");
    this.properties = { format: "YYYY-MM-DD HH:mm:ss" };
    this.addWidget("text", "格式", "YYYY-MM-DD HH:mm:ss", { property: "format" });
    this.boxcolor = "#936";
}
TimeFormatNode.title = "格式化时间";
TimeFormatNode.desc = "将时间戳格式化为日期时间字符串";
TimeFormatNode.prototype.onExecute = function() {
    var d = new Date(this.getInputData(0) || Date.now());
    var fmt = this.properties.format || "YYYY-MM-DD HH:mm:ss";
    this.setOutputData(0, fmt
        .replace("YYYY", d.getFullYear())
        .replace("MM", String(d.getMonth()+1).padStart(2,"0"))
        .replace("DD", String(d.getDate()).padStart(2,"0"))
        .replace("HH", String(d.getHours()).padStart(2,"0"))
        .replace("mm", String(d.getMinutes()).padStart(2,"0"))
        .replace("ss", String(d.getSeconds()).padStart(2,"0")));
};
LiteGraph.registerNodeType("time/format", TimeFormatNode);

function TimeCalculateNode() {
    this.addInput("时间戳(ms)", "number");
    this.addOutput("结果时间戳", "number");
    this.properties = { operation: "add", amount: 1, unit: "days" };
    this.addWidget("combo", "操作", "add", { property: "operation", values: ["add", "subtract"] });
    this.addWidget("number", "数量", 1, { property: "amount" });
    this.addWidget("combo", "单位", "days", { property: "unit", values: ["seconds", "minutes", "hours", "days"] });
    this.boxcolor = "#936";
}
TimeCalculateNode.title = "时间计算";
TimeCalculateNode.desc = "对时间戳进行加减运算";
TimeCalculateNode.prototype.onExecute = function() {
    var ts = this.getInputData(0) || Date.now();
    var ms = {seconds:1000, minutes:60000, hours:3600000, days:86400000}[this.properties.unit||"days"]||86400000;
    var delta = (this.properties.amount||0) * ms;
    this.setOutputData(0, this.properties.operation === "subtract" ? ts - delta : ts + delta);
};
LiteGraph.registerNodeType("time/calculate", TimeCalculateNode);

function DelayNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { delay: 1000 };
    this.addWidget("number", "延时(毫秒)", 1000, { property: "delay" });
    this.boxcolor = "#936";
}
DelayNode.title = "延时执行";
DelayNode.desc = "延迟指定时间后继续执行";
DelayNode.prototype.onAction = function(action, param) {
    var that = this;
    setTimeout(function() { that.triggerSlot(0, param); }, this.properties.delay || 1000);
};
LiteGraph.registerNodeType("time/delay", DelayNode);

// ==================== 日志节点 ====================

function LogDebugNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("消息", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { message: "" };
    this.addWidget("text", "消息", "", { property: "message", multiline: true });
    this.boxcolor = "#2a9";
}
LogDebugNode.title = "输出日志";
LogDebugNode.desc = "输出调试级别的日志信息";
LiteGraph.registerNodeType("log/debug", LogDebugNode);

// ==================== 时间节点 ====================

function TimestampNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("时间戳", "number");
    this.boxcolor = "#29a";
}
TimestampNode.title = "13位时间戳";
TimestampNode.desc = "获取当前时间的13位毫秒时间戳";
LiteGraph.registerNodeType("time/timestamp", TimestampNode);

function TimeFormatCustomNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("时间戳", "number");
    this.addOutput("格式化时间", "string");
    this.properties = { format: "yyyy-MM-dd hh:mm:ss" };
    this.addWidget("text", "日期时间格式", "yyyy-MM-dd hh:mm:ss", { property: "format" });
    this.boxcolor = "#29a";
}
TimeFormatCustomNode.title = "日期时间格式化";
TimeFormatCustomNode.desc = "将时间戳格式化为指定的日期时间字符串";
LiteGraph.registerNodeType("time/format_custom", TimeFormatCustomNode);

// ==================== 代码节点 ====================

function CodeGoNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { code: "// 在这里写 Go 代码\n// 可用变量: message, userID, groupID, eventType, inputs, sctx\n// 设置 output[\"key\"] = value 来返回输出\noutput[\"hello\"] = \"你好, \" + userID;\n" };
    this.addWidget("text", "Go代码", "", { property: "code" });
    this.boxcolor = "#d94a90";
}
CodeGoNode.title = "Go代码";
CodeGoNode.desc = "在流程图中直接执行 Go 代码片段（基于 yaegi 解释器）";
LiteGraph.registerNodeType("code/go", CodeGoNode);

// ==================== 上下文信息节点 ====================

function GetPluginIDNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("插件ID", "string");
    this.boxcolor = "#636";
}
GetPluginIDNode.title = "当前插件ID";
GetPluginIDNode.desc = "获取当前执行的插件ID";
LiteGraph.registerNodeType("context/get_plugin_id", GetPluginIDNode);

// ==================== 列表节点 ====================

function ListCreateEmptyNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("列表ID", "string");
    this.properties = { list_id: "", name: "" };
    this.addWidget("text", "列表ID(可选)", "", { property: "list_id" });
    this.addWidget("text", "列表名称", "", { property: "name" });
    this.boxcolor = "#936";
}
ListCreateEmptyNode.title = "创建空列表";
ListCreateEmptyNode.desc = "创建一个空的持久化列表";
LiteGraph.registerNodeType("list/create_empty", ListCreateEmptyNode);

function ListCreateNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("列表ID", "string");
    this.properties = { list_id: "", name: "", content: "" };
    this.addWidget("text", "列表ID(可选)", "", { property: "list_id" });
    this.addWidget("text", "列表名称", "", { property: "name" });
    this.addWidget("text", "内容(逗号分隔)", "", { property: "content" });
    this.boxcolor = "#936";
}
ListCreateNode.title = "创建列表";
ListCreateNode.desc = "创建一个包含初始内容的持久化列表";
LiteGraph.registerNodeType("list/create", ListCreateNode);

function ListAddRepeatNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { list_id: "", value: "", count: 5 };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.addWidget("text", "重复项", "", { property: "value" });
    this.addWidget("number", "重复次数", 5, { property: "count" });
    this.boxcolor = "#936";
}
ListAddRepeatNode.title = "添加重复项";
ListAddRepeatNode.desc = "向列表中添加重复的项";
LiteGraph.registerNodeType("list/add_repeat", ListAddRepeatNode);

function ListLengthNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addOutput("长度", "number");
    this.properties = { list_id: "" };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.boxcolor = "#936";
}
ListLengthNode.title = "列表长度";
ListLengthNode.desc = "获取列表的长度";
LiteGraph.registerNodeType("list/length", ListLengthNode);

function ListIsEmptyNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addOutput("结果", "boolean");
    this.properties = { list_id: "" };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.boxcolor = "#936";
}
ListIsEmptyNode.title = "列表是否为空";
ListIsEmptyNode.desc = "判断列表是否为空";
LiteGraph.registerNodeType("list/is_empty", ListIsEmptyNode);

function ListFindNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addInput("查找内容", "string");
    this.addOutput("索引", "number");
    this.properties = { list_id: "", value: "" };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.addWidget("text", "查找内容", "", { property: "value" });
    this.boxcolor = "#936";
}
ListFindNode.title = "查找元素";
ListFindNode.desc = "在列表中查找第一次出现的元素，返回索引";
LiteGraph.registerNodeType("list/find", ListFindNode);

function ListGetNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addInput("索引", "number");
    this.addOutput("值", "string");
    this.properties = { list_id: "" };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.boxcolor = "#936";
}
ListGetNode.title = "获取元素";
ListGetNode.desc = "获取列表中指定索引的元素";
LiteGraph.registerNodeType("list/get", ListGetNode);

function ListSetNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addInput("索引", "number");
    this.addInput("值", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { list_id: "", value: "" };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.addWidget("text", "值", "", { property: "value" });
    this.boxcolor = "#936";
}
ListSetNode.title = "设置元素";
ListSetNode.desc = "设置列表中指定索引的元素";
LiteGraph.registerNodeType("list/set", ListSetNode);

function ListSubListNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addInput("起始索引", "number");
    this.addInput("结束索引", "number");
    this.addOutput("子列表", "array");
    this.properties = { list_id: "" };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.boxcolor = "#936";
}
ListSubListNode.title = "获取子列表";
ListSubListNode.desc = "获取列表的子列表";
LiteGraph.registerNodeType("list/sublist", ListSubListNode);

function ListFromTextNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("文本", "string");
    this.addOutput("列表", "array");
    this.properties = { text: "", delimiter: "," };
    this.addWidget("text", "文本", "", { property: "text", multiline: true });
    this.addWidget("text", "分隔符", ",", { property: "delimiter" });
    this.boxcolor = "#936";
}
ListFromTextNode.title = "从文本制作列表";
ListFromTextNode.desc = "将文本按分隔符分割为列表";
LiteGraph.registerNodeType("list/from_text", ListFromTextNode);

function ListSortNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { list_id: "", by_number: false, ascending: true };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.addWidget("toggle", "按数字排序", false, { property: "by_number" });
    this.addWidget("toggle", "升序", true, { property: "ascending" });
    this.boxcolor = "#936";
}
ListSortNode.title = "排序";
ListSortNode.desc = "对列表进行排序";
LiteGraph.registerNodeType("list/sort", ListSortNode);

function ListDeleteNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("列表ID", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { list_id: "" };
    this.addWidget("text", "列表ID", "", { property: "list_id" });
    this.boxcolor = "#936";
}
ListDeleteNode.title = "删除列表";
ListDeleteNode.desc = "删除列表及其所有数据";
LiteGraph.registerNodeType("list/delete", ListDeleteNode);

// ==================== 指令节点 ====================

function CreateCommandNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addInput("指令名", "string");
    this.addInput("指令描述", "string");
    this.addOutput("完成", LiteGraph.EVENT);
    this.properties = { name: "", description: "", usage: "", aliases: "" };
    this.addWidget("text", "指令名", "", { property: "name" });
    this.addWidget("text", "描述", "", { property: "description", multiline: true });
    this.addWidget("text", "使用方法", "", { property: "usage", multiline: true });
    this.addWidget("text", "别名(逗号分隔)", "", { property: "aliases" });
    this.boxcolor = "#963";
}
CreateCommandNode.title = "创建指令";
CreateCommandNode.desc = "注册一个机器人指令，如 /help、/ping";
LiteGraph.registerNodeType("command/create", CreateCommandNode);

function PermissionCheckNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("有权限", LiteGraph.EVENT);
    this.addOutput("无权限", LiteGraph.EVENT);
    this.addOutput("结果", "boolean");
    this.properties = { permission: "admin" };
    this.addWidget("combo", "权限级别", "admin", { property: "permission", values: ["admin", "owner", "member"] });
    this.boxcolor = "#963";
}
PermissionCheckNode.title = "权限检查";
PermissionCheckNode.desc = "检查当前用户是否有指定权限";
PermissionCheckNode.prototype.onAction = function(action, param) {
    var result = true;
    this.setOutputData(2, result);
    if (result) this.triggerSlot(0, param);
    else this.triggerSlot(1, param);
};
LiteGraph.registerNodeType("command/permission", PermissionCheckNode);

// ==================== 上下文信息节点 ====================

function GetUserIDNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("发送者ID", "string");
    this.boxcolor = "#396";
}
GetUserIDNode.title = "获取发送者ID";
GetUserIDNode.desc = "获取当前消息发送者的用户ID";
LiteGraph.registerNodeType("context/get_user_id", GetUserIDNode);

function GetGroupIDNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("群聊ID", "string");
    this.boxcolor = "#396";
}
GetGroupIDNode.title = "获取群聊ID";
GetGroupIDNode.desc = "获取当前消息来源的群聊ID（仅群聊消息有效）";
LiteGraph.registerNodeType("context/get_group_id", GetGroupIDNode);

function GetChannelIDNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("频道ID", "string");
    this.boxcolor = "#396";
}
GetChannelIDNode.title = "获取频道ID";
GetChannelIDNode.desc = "获取当前消息来源的频道ID（仅频道消息有效）";
LiteGraph.registerNodeType("context/get_channel_id", GetChannelIDNode);

function GetGuildIDNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("GuildID", "string");
    this.boxcolor = "#396";
}
GetGuildIDNode.title = "获取GuildID";
GetGuildIDNode.desc = "获取当前消息来源的GuildID（仅频道消息有效）";
LiteGraph.registerNodeType("context/get_guild_id", GetGuildIDNode);

function GetMessageIDNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("消息ID", "string");
    this.boxcolor = "#396";
}
GetMessageIDNode.title = "获取消息ID";
GetMessageIDNode.desc = "获取当前触发消息的ID，可用于撤回等操作";
LiteGraph.registerNodeType("context/get_message_id", GetMessageIDNode);

function GetSourceNode() {
    this.addInput("触发", LiteGraph.ACTION);
    this.addOutput("来源类型", "string");
    this.addOutput("来源ID", "string");
    this.boxcolor = "#396";
}
GetSourceNode.title = "获取消息来源";
GetSourceNode.desc = "获取消息来源类型（群聊/私聊/频道）和来源ID";
LiteGraph.registerNodeType("context/get_source", GetSourceNode);

// ==================== 多行 widget 渲染支持 ====================
// 背景：LiteGraph 的 text widget 默认把值用 substr(0,30) 单行右对齐渲染，
// 且 prompt 对话框在未传 multiline 时走单行 <input>（回车即提交，无法换行）。
// 上面已给"内容型"widget 的 options 加了 multiline:true，使输入用 <textarea>
// 并把真实换行存进 properties（后端 getStrProp 透传，无需改动）。
// 这里覆写 drawNodeWidgets：对多行内容，用"首行 + (N行)"提示替换原 30 字符截断显示，
// 节点高度不变（仍按固定 NODE_WIDGET_HEIGHT 排版）。
(function () {
    if (typeof LGraphCanvas === "undefined" || !LGraphCanvas.prototype.drawNodeWidgets) return;
    var _origDrawNodeWidgets = LGraphCanvas.prototype.drawNodeWidgets;
    LGraphCanvas.prototype.drawNodeWidgets = function (node, posY, ctx, active_widget) {
        var result = _origDrawNodeWidgets.call(this, node, posY, ctx, active_widget);
        // 缩放过小时原函数本身就只画框不画字，这里也跳过
        if (this.ds.scale <= 0.5) return result;
        if (!node.widgets) return result;
        var margin = 15;
        var H = LiteGraph.NODE_WIDGET_HEIGHT;
        var widget_width_base = node.size[0];
        for (var i = 0; i < node.widgets.length; ++i) {
            var w = node.widgets[i];
            if (w.type !== "text") continue;
            if (!w.options || !w.options.multiline) continue;
            if (w.value == null) continue;
            if (w.last_y == null) continue;
            var val = String(w.value);
            var lines = val.split("\n");
            if (lines.length <= 1) continue; // 单行走原渲染逻辑即可
            var widget_width = w.width || widget_width_base;
            var y = w.last_y;
            ctx.save();
            // 用 widget 背景色覆盖原值文本区域（原函数已画过 substr(0,30) 的值）
            ctx.beginPath();
            ctx.rect(margin, y, widget_width - margin * 2, H);
            ctx.clip();
            ctx.fillStyle = LiteGraph.WIDGET_BGCOLOR;
            ctx.fillRect(margin, y, widget_width - margin * 2, H);
            // 重画：首行（截断）+ (N行) 提示，右对齐，与原值文本同色同基线
            ctx.fillStyle = LiteGraph.WIDGET_TEXT_COLOR;
            ctx.textAlign = "right";
            var firstLine = lines[0].substr(0, 24);
            ctx.fillText(firstLine + " (" + lines.length + "行)", widget_width - margin * 2, y + H * 0.7);
            ctx.restore();
        }
        return result;
    };
})();

console.log("[PluginNodes] ShizukuBot 插件节点已注册");