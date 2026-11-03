// ShizukuBot WebUI 应用逻辑 - MDUI2

// ==================== 全局状态 ====================
let currentRoute = '/stats';
let statsRefreshTimer = null;
let logWebSocket = null;
let historyChart = null;
let hourlyChart = null;
let historyPage = 1;
const historyPageSize = 50;

// API 接口列表
const apiList = [
    { method: 'GET',    path: '/api/bots',                      desc: '获取所有机器人列表' },
    { method: 'GET',    path: '/api/bots/{id}',                 desc: '获取单个机器人信息' },
    { method: 'POST',   path: '/api/bots',                      desc: '创建新机器人' },
    { method: 'PUT',    path: '/api/bots/{id}',                 desc: '更新机器人配置' },
    { method: 'DELETE', path: '/api/bots/{id}',                 desc: '删除机器人' },
    { method: 'POST',   path: '/api/bots/{id}/start',           desc: '启动机器人' },
    { method: 'POST',   path: '/api/bots/{id}/stop',            desc: '停止机器人' },
    { method: 'POST',   path: '/api/bots/{id}/message/group',   desc: '发送群消息' },
    { method: 'POST',   path: '/api/bots/{id}/message/c2c',     desc: '发送C2C消息' },
    { method: 'GET',    path: '/api/stats',                     desc: '获取汇总统计数据' },
    { method: 'GET',    path: '/api/bots/{id}/stats',           desc: '获取单个机器人统计' },
    { method: 'GET',    path: '/api/config',                    desc: '获取当前配置' },
    { method: 'PUT',    path: '/api/config',                    desc: '更新配置' },
    { method: 'GET',    path: '/api/logs',                      desc: '获取历史日志' },
];

// ==================== API / page lifecycle ====================
let pageScope = null;
let pageEpoch = 0;
let navigationEpoch = 0;
async function apiRequest(url, options = {}) {
    const response = await fetch(url, options);
    const data = await response.json();
    if (options.signal?.aborted) throw new DOMException('页面已切换', 'AbortError');
    if (!response.ok || (typeof data.code === 'number' && data.code !== 0)) {
        const error = Error(data.message || `HTTP ${response.status}`); error.status = response.status; throw error;
    }
    return data;
}
async function apiGet(url) {return apiRequest(url, {signal: pageScope?.controller.signal});}
async function apiPost(url, data) {return apiRequest(url, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data)});}
async function apiPut(url, data) {return apiRequest(url, {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data)});}
async function apiDelete(url) {return apiRequest(url, {method: 'DELETE'});}

// ==================== 路由管理 ====================
async function navigateTo(route) {
    if (route === currentRoute && pageScope) {syncNavigation(route); return true;}
    const token = ++navigationEpoch;
    const allowed = await window.PluginUI.mayLeave();
    if (token !== navigationEpoch) return false;
    if (!allowed) {
        history.replaceState(null, '', '#' + currentRoute); syncNavigation(currentRoute); return false;
    }
    currentRoute = route;
    if (window.location.hash !== '#' + route) history.pushState(null, '', '#' + route);
    renderPage(); syncNavigation(route); return true;
}

function syncNavigation(route) {
    // 更新移动端导航项激活状态
    document.querySelectorAll('.nav-item').forEach(item => {
        item.active = item.dataset.route === route;
        if (item.active) item.setAttribute('aria-current', 'page'); else item.removeAttribute('aria-current');
    });

    // 更新桌面端 NavigationRail 选中项
    const rail = document.querySelector('.desktop-nav');
    if (rail) {
        rail.querySelectorAll('[data-route]').forEach(item => {if (item.dataset.route === route) item.setAttribute('aria-current', 'page'); else item.removeAttribute('aria-current');});
        const routeMap = {
            '/stats': 'stats',
            '/bots': 'bots',
            '/logs': 'logs',
            '/api-test': 'api-test',
            '/plugins': 'plugins',
            '/rules': 'rules',
            '/community': 'community',
            '/sandbox': 'sandbox',
            '/config': 'config',
            '/users': 'users',
            '/about': 'about',
        };
        rail.value = routeMap[route] || 'stats';
    }
}

function renderPage() {
    const content = document.getElementById('app-content');
    const previous = content.querySelector(':scope > .page-view:not([hidden])');
    if (previous) {previous.hidden = true; previous.inert = true;}
    const view = document.createElement('div'); view.className = 'page-view';
    content.prepend(view);
    ++pageEpoch;
    if (statsRefreshTimer) {clearInterval(statsRefreshTimer); statsRefreshTimer = null;}
    if (logWebSocket) {logWebSocket.onmessage = null; logWebSocket.close(); logWebSocket = null;}
    historyChart?.destroy(); hourlyChart?.destroy(); historyChart = hourlyChart = null;
    pageScope = window.AdminUI.begin(view);
    const renderers = {'/stats': renderStats, '/bots': renderBots, '/logs': renderLogs, '/api-test': renderApiTest,
        '/plugins': window.PluginPages.renderPlugins, '/rules': window.RulesPage.render,
        '/community': renderCommunity, '/sandbox': window.SandboxPage.render, '/config': renderConfig,
        '/users': renderUsers, '/about': renderAbout};
    const epoch = pageEpoch;
    Promise.resolve((renderers[currentRoute] || renderStats)(view)).catch(error => {
        if (epoch === pageEpoch && error.name !== 'AbortError') showSnackbar('页面加载失败：' + error.message);
    });
    window.AdminUI.enter(content, true);
    // MDUI 2 text fields await their own updateComplete before sizing. Keep the
    // previous view connected but hidden through native updates and the next frame.
    // This does not delay the new renderer or its requests, and uses no timer.
    if (previous) {
        const fields = [...previous.querySelectorAll('mdui-text-field')];
        Promise.allSettled(fields.map(async field => {
            do {await field.updateComplete;} while (field.isUpdatePending);
        })).then(() => requestAnimationFrame(() => previous.remove()));
    }
}

// ==================== 统计页 ====================
async function renderStats(container) {
    const epoch = pageEpoch;
    container.innerHTML = `
        <div class="page-title">统计</div>
        <p id="stats-window" class="plugin-muted"></p>
        <div id="stats-feedback" aria-live="polite"></div>

        <!-- 机器人管理 -->
        <mdui-card variant="outlined" class="bots-card">
            <div style="font-size: 16px; font-weight: 500; margin-bottom: 12px; display: flex; align-items: center; gap: 8px;">
                <mdui-icon name="smart_toy"></mdui-icon> 机器人管理            </div>
            <div id="bots-list"></div>
        </mdui-card>

        <!-- 统计卡片 -->
        <div class="stats-grid">
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="group"></mdui-icon> 用户数量</div>
                <div class="stat-card-value" id="stat-users">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="groups"></mdui-icon> 群组数量</div>
                <div class="stat-card-value" id="stat-groups">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="today"></mdui-icon> 今日DAU</div>
                <div class="stat-card-value" id="stat-dau">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="speed"></mdui-icon> 当前QPS</div>
                <div class="stat-card-value" id="stat-qps">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="call_received"></mdui-icon> 接收消息</div>
                <div class="stat-card-value" id="stat-received">-</div>
            </mdui-card>
            <mdui-card variant="outlined" class="stat-card">
                <div class="stat-card-label"><mdui-icon name="send"></mdui-icon> 发送消息</div>
                <div class="stat-card-value" id="stat-sent">-</div>
            </mdui-card>
        </div>

        <!-- 图表 -->
        <div class="charts-grid">
            <mdui-card variant="outlined" class="chart-card">
                <div class="chart-heading"><span>历史消息数量</span><mdui-select id="history-range" value="30" aria-label="历史范围">
                    <mdui-menu-item value="7">最近 7 天</mdui-menu-item><mdui-menu-item value="30">最近 30 天</mdui-menu-item><mdui-menu-item value="90">最近 90 天</mdui-menu-item>
                </mdui-select></div>
                <div class="chart-container"><canvas id="history-chart"></canvas></div>
            </mdui-card>
            <mdui-card variant="outlined" class="chart-card">
                <div style="font-size: 16px; font-weight: 500; margin-bottom: 12px">每小时消息数量</div>
                <div class="chart-container"><canvas id="hourly-chart"></canvas></div>
            </mdui-card>
        </div>
    `;

    await loadBotsList();
    if (epoch !== pageEpoch) return;
    initHistoryChart();
    initHourlyChart();
    pageScope.on(document.getElementById('history-range'),'change',updateStats);
    pageScope.on(matchMedia('(prefers-color-scheme: dark)'),'change',recolorCharts);
    pageScope.on(window,'appearancechange',recolorCharts);
    await updateStats();
    if (epoch !== pageEpoch) return;
    statsRefreshTimer = setInterval(() => {
        updateStats();
        if (!document.querySelector('#bots-list mdui-button[loading]')) loadBotsList();
    }, 5000);
}

async function loadBotsList() {
    try {
        const data = await apiGet('/api/bots');
        const listEl = document.getElementById('bots-list');
        if (!listEl) return;

        if (data.code !== 0 || !data.data || data.data.length === 0) {
            listEl.innerHTML = '<div style="padding:16px;color:rgb(var(--mdui-color-on-surface-variant))">暂无机器人，请在配置页添加</div>';
            return;
        }

        listEl.innerHTML = data.data.map(bot => {
                const statusColor = bot.running ? '#4caf50' : '#9e9e9e';
                const statusText = bot.running ? '运行中' : '已停止';
                const modeText = bot.mode === 'websocket' ? 'WebSocket' : 'Webhook';
                const modeColor = bot.mode === 'websocket' ? '#1976d2' : '#f57c00';
                const btnText = bot.running ? '停止' : '启动';
                const btnIcon = bot.running ? 'stop' : 'play_arrow';
                const btnVariant = bot.running ? 'outlined' : 'filled';
                const btnAction = bot.running ? 'stop' : 'start';

                return `
                    <div class="bot-item" data-bot-id="${escapeHtml(bot.id)}">
                        <div class="bot-item-info">
                            <span class="bot-status-dot" style="background:${statusColor}"></span>
                            <div class="bot-item-text">
                                <div class="bot-item-name">${escapeHtml(bot.name || bot.id)}</div>
                                <div class="bot-item-meta">
                                    ID: ${escapeHtml(bot.id)} · AppID: ${escapeHtml(bot.app_id)} · 
                                    <span class="bot-mode" style="color:${modeColor}">${modeText}</span> · ${statusText}
                                </div>
                            </div>
                        </div>
                        <mdui-button variant="${btnVariant}" icon="${btnIcon}" data-action="${btnAction}" data-id="${escapeHtml(bot.id)}">
                            ${btnText}
                        </mdui-button>
                    </div>
                `;
            }).join('');

        // 绑定按钮事件
        listEl.querySelectorAll('[data-action]').forEach(btn => {
            btn.addEventListener('click', async () => {
                const id = btn.dataset.id;
                const action = btn.dataset.action;
                if (btn.loading) return;
                btn.loading = true; btn.disabled = true;
                try {
                    if (action === 'start') {
                        await apiPost(`/api/bots/${id}/start`, {});
                    } else {
                        await apiPost(`/api/bots/${id}/stop`, {});
                    }
                    showSnackbar(action === 'start' ? '已启动' : '已停止');
                    await loadBotsList();
                } catch (e) {
        if (e.name === 'AbortError') return;
                    showSnackbar('操作失败: ' + e.message);
                }
                btn.loading = false; btn.disabled = false;
            });
        });
    } catch (e) {
        if (e.name === 'AbortError') return;
        console.error('加载机器人列表失败', e);
    }
}

async function updateStats() {
    try {
        const epoch=pageEpoch;
        const days=document.getElementById('history-range')?.value||'30';
        const [data,history]=await Promise.all([apiGet('/api/stats'),apiGet('/api/stats/history?days='+encodeURIComponent(days))]);
        if(epoch!==pageEpoch)return;
        if (data.code === 0 && data.data) {
            const s = data.data;
            document.getElementById('stats-window').textContent=(s.retention_days ? `最近 ${s.retention_days} 天`:'永久保留')+' · '+(s.timezone||'本地时区')+' · 各机器人分别去重后求和';
            AdminUI.notice(document.getElementById('stats-feedback'),'');
            if(historyChart){const points=history.data||[];historyChart.data.labels=points.map(p=>p.date);historyChart.data.datasets[0].data=points.map(p=>p.sent);historyChart.data.datasets[1].data=points.map(p=>p.received);historyChart.update('none');}
            document.getElementById('stat-users').textContent = s.user_count || 0;
            document.getElementById('stat-groups').textContent = s.group_count || 0;
            document.getElementById('stat-dau').textContent = s.today_dau || 0;
            document.getElementById('stat-qps').textContent = s.qps || 0;
            document.getElementById('stat-received').textContent = s.received_messages || 0;
            document.getElementById('stat-sent').textContent = s.sent_messages || 0;

            if (s.hourly_stats && hourlyChart) {
                const sentData = s.hourly_stats.map(h => h.sent || 0);
                const receivedData = s.hourly_stats.map(h => h.received || 0);
                hourlyChart.data.datasets[0].data = sentData;
                hourlyChart.data.datasets[1].data = receivedData;
                hourlyChart.update('none');
            }
        }
    } catch (e) {
        if (e.name === 'AbortError') return;
        const host=document.getElementById('stats-feedback');if(host)AdminUI.notice(host,'统计加载失败：'+e.message,'error');
    }
}

function chartPalette(){const style=getComputedStyle(document.documentElement);const color=name=>'rgb('+style.getPropertyValue('--mdui-color-'+name).trim()+')';return {sent:color('primary'),received:color('tertiary'),text:color('on-surface-variant'),grid:color('outline-variant')};}
function chartOptions(){const palette=chartPalette();return {responsive:true,maintainAspectRatio:false,
    animation:matchMedia('(prefers-reduced-motion: reduce)').matches?false:{duration:250},
    plugins:{legend:{labels:{color:palette.text}}},scales:{x:{ticks:{color:palette.text},grid:{color:palette.grid}},y:{beginAtZero:true,ticks:{color:palette.text,precision:0},grid:{color:palette.grid}}}};}
function recolorCharts(){
    const palette=chartPalette();
    for(const chart of [historyChart,hourlyChart]){if(!chart)continue;
        chart.data.datasets.forEach((dataset,index)=>{dataset.borderColor=index?palette.received:palette.sent;dataset.backgroundColor=index?palette.received:palette.sent;});
        chart.options.plugins.legend.labels.color=palette.text;
        for(const axis of Object.values(chart.options.scales)){axis.ticks.color=palette.text;axis.grid.color=palette.grid;}
        chart.update('none');
    }
}
function initHistoryChart() {
    const ctx = document.getElementById('history-chart');
    if (!ctx) return;
    historyChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [
                { label: '发送', data: [], borderColor: chartPalette().sent, tension: 0.3, fill: false },
                { label: '接收', data: [], borderColor: chartPalette().received, tension: 0.3, fill: false },
            ],
        },
        options: chartOptions(),
    });
}

function initHourlyChart() {
    const ctx = document.getElementById('hourly-chart');
    if (!ctx) return;
    hourlyChart = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: Array.from({ length: 24 }, (_, i) => i + ':00'),
            datasets: [
                { label: '发送', data: Array(24).fill(0), backgroundColor: chartPalette().sent },
                { label: '接收', data: Array(24).fill(0), backgroundColor: chartPalette().received },
            ],
        },
        options: chartOptions(),
    });
}

// ==================== 日志页 ====================
function renderLogs(container) {
    container.innerHTML = `
        <div class="page-title">日志</div>
        <div class="logs-controls">
            <mdui-text-field label="搜索" icon="search" id="log-search"></mdui-text-field>
            <mdui-select value="all" id="log-filter">
                <mdui-menu-item value="all">全部</mdui-menu-item>
                <mdui-menu-item value="debug">Debug</mdui-menu-item>
                <mdui-menu-item value="info">Info</mdui-menu-item>
                <mdui-menu-item value="warn">Warn</mdui-menu-item>
                <mdui-menu-item value="error">Error</mdui-menu-item>
            </mdui-select>
            <mdui-button variant="tonal" icon="refresh" id="log-refresh">刷新</mdui-button>
        </div>
        <div class="logs-container">
            <mdui-card variant="outlined" class="log-card">
                <div class="log-card-header">
                    <span class="log-card-title">实时日志</span>
                    <mdui-chip icon="radio_button_checked" selected>实时</mdui-chip>
                </div>
                <div class="logs-area" id="realtime-logs"></div>
            </mdui-card>
            <mdui-card variant="outlined" class="log-card">
                <div class="log-card-header">
                    <span class="log-card-title">历史日志</span>
                </div>
                <div class="logs-area" id="history-logs"></div>
                <div class="pagination" id="log-pagination"></div>
            </mdui-card>
        </div>
    `;

    initRealtimeLogs();
    loadHistoryLogs();

    document.getElementById('log-refresh').addEventListener('click', loadHistoryLogs);
    document.getElementById('log-filter').addEventListener('change', () => {
        historyPage = 1;
        loadHistoryLogs();
    });
    document.getElementById('log-search').addEventListener('input', () => {
        historyPage = 1;
        loadHistoryLogs();
    });
}

function initRealtimeLogs() {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/ws/logs`;
    logWebSocket = new WebSocket(wsUrl);

    const area = document.getElementById('realtime-logs');

    logWebSocket.onmessage = (event) => {
        const log = JSON.parse(event.data);
        appendLogItem(area, log);
        // 限制最多200条
        while (area.children.length > 200) {
            area.removeChild(area.firstChild);
        }
    };

    logWebSocket.onerror = (e) => console.error('WebSocket错误:', e);
}

function appendLogItem(container, log) {
    const item = document.createElement('div');
    item.className = 'log-item';
    const time = new Date(log.timestamp).toLocaleTimeString('zh-CN');
    const levelClass = `log-level-${log.level.toLowerCase()}`;
    item.innerHTML = `
        <span class="log-time">${time}</span>
        <span class="log-level ${levelClass}">[${log.level}]</span>
        <span class="log-message">${escapeHtml(log.message)}</span>
    `;
    container.appendChild(item);
    container.scrollTop = container.scrollHeight;
}

async function loadHistoryLogs() {
    const filter = document.getElementById('log-filter');
    const search = document.getElementById('log-search');
    const level = filter ? filter.value : 'all';
    const keyword = search ? search.value : '';
    const offset = (historyPage - 1) * historyPageSize;

    try {
        const params = new URLSearchParams({
            limit: historyPageSize,
            offset: offset,
        });
        if (level !== 'all') params.set('level', level);
        if (keyword) params.set('keyword', keyword);

        const data = await apiGet(`/api/logs?${params.toString()}`);
        const area = document.getElementById('history-logs');
        area.innerHTML = '';

        if (data.code === 0 && data.data && data.data.logs) {
            data.data.logs.forEach(log => appendLogItem(area, log));
        } else {
            area.innerHTML = '<div style="padding:16px;color:rgb(var(--mdui-color-on-surface-variant))">暂无日志</div>';
        }

        renderPagination(data.data?.total || 0);
    } catch (e) {
        if (e.name === 'AbortError') return;
        console.error('加载历史日志失败:', e);
    }
}

function renderPagination(total) {
    const container = document.getElementById('log-pagination');
    const totalPages = Math.ceil(total / historyPageSize);
    if (totalPages <= 1) {
        container.innerHTML = '';
        return;
    }

    let html = '';
    if (historyPage > 1) {
        html += `<mdui-button-icon icon="chevron_left" data-page="${historyPage - 1}"></mdui-button-icon>`;
    }
    html += `<span style="padding:0 12px">${historyPage} / ${totalPages}</span>`;
    if (historyPage < totalPages) {
        html += `<mdui-button-icon icon="chevron_right" data-page="${historyPage + 1}"></mdui-button-icon>`;
    }
    container.innerHTML = html;

    container.querySelectorAll('[data-page]').forEach(btn => {
        btn.addEventListener('click', () => {
            historyPage = parseInt(btn.dataset.page);
            loadHistoryLogs();
        });
    });
}

// ==================== 接口调试页 ====================
function renderApiTest(container) {
    container.innerHTML = `
        <div class="page-title">接口调试</div>
        <div class="api-test-container">
            <mdui-card variant="outlined" class="api-list-card">
                <div class="api-list-title">API 列表</div>
                <mdui-list id="api-list">
                    ${apiList.map((api, i) => `
                        <mdui-list-item
                            headline="${api.method} ${api.path}"
                            description="${api.desc}"
                            data-index="${i}"
                            class="api-list-item">
                        </mdui-list-item>
                    `).join('')}
                </mdui-list>
            </mdui-card>
            <div class="api-form">
                <mdui-card variant="outlined" class="api-form-card">
                    <div style="font-size:16px;font-weight:500;margin-bottom:16px">请求</div>
                    <div class="api-form-row">
                        <mdui-segmented-button-group selects="single" id="api-method">
                            <mdui-segmented-button value="GET">GET</mdui-segmented-button>
                            <mdui-segmented-button value="POST">POST</mdui-segmented-button>
                            <mdui-segmented-button value="PUT">PUT</mdui-segmented-button>
                            <mdui-segmented-button value="DELETE">DELETE</mdui-segmented-button>
                        </mdui-segmented-button-group>
                    </div>
                    <div class="api-form-row">
                        <mdui-text-field label="路径" id="api-path" full-width></mdui-text-field>
                    </div>
                    <div class="api-form-row">
                        <mdui-text-field
                            label="请求体 (JSON)"
                            id="api-body"
                            full-width
                            rows="4"></mdui-text-field>
                    </div>
                    <mdui-button variant="filled" icon="send" id="api-send">发送请求</mdui-button>
                </mdui-card>
                <mdui-card variant="outlined" class="api-form-card">
                    <div style="font-size:16px;font-weight:500;margin-bottom:16px">响应</div>
                    <pre class="response-pre" id="api-response">点击"发送请求"查看结果</pre>
                </mdui-card>
            </div>
        </div>
    `;

    // 默认选中第一个接口
    const methodGroup = document.getElementById('api-method');
    methodGroup.value = 'GET';
    document.getElementById('api-path').value = '/api/bots';

    document.querySelectorAll('.api-list-item').forEach(item => {
        item.addEventListener('click', () => {
            const idx = parseInt(item.dataset.index);
            const api = apiList[idx];
            methodGroup.value = api.method;
            document.getElementById('api-path').value = api.path;
        });
    });

    document.getElementById('api-send').addEventListener('click', sendApiRequest);
}

async function sendApiRequest() {
    const method = document.getElementById('api-method').value;
    const path = document.getElementById('api-path').value;
    const bodyText = document.getElementById('api-body').value;
    const respArea = document.getElementById('api-response');

    respArea.textContent = '请求中...';

    try {
        const opts = { method, signal: pageScope.controller.signal };
        if (bodyText && (method === 'POST' || method === 'PUT')) {
            opts.headers = { 'Content-Type': 'application/json' };
            opts.body = bodyText;
        }
        const resp = await fetch(path, opts);
        const text = await resp.text();
        try {
            const json = JSON.parse(text);
            respArea.textContent = JSON.stringify(json, null, 2);
        } catch {
            respArea.textContent = text;
        }
    } catch (e) {
        if (e.name === 'AbortError') return;
        respArea.textContent = '请求失败: ' + e.message;
    }
}

// ==================== 配置页 ====================
async function renderConfig(container) {
    const epoch = pageEpoch;
    container.innerHTML = `
        <div class="page-title">配置</div>
        <div id="preferences-settings"></div>

        <div id="config-feedback" aria-live="polite"></div>
        <!-- WebUI 配置 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="font-size:16px;font-weight:500;margin-bottom:16px">WebUI 设置</div>
            <div style="display:flex;gap:16px;flex-wrap:wrap;margin-bottom:16px">
                <mdui-text-field label="端口" type="number" min="1" max="65535" id="webui-port" aria-describedby="config-port-error" style="width:200px"></mdui-text-field>
                <mdui-switch label="启用" id="webui-enabled"></mdui-switch>
            </div>
            <div id="config-port-error" class="field-error" role="alert"></div>
            <mdui-text-field label="WebUI 密钥" type="password" id="webui-password" help-text="留空保留原值" toggle-password></mdui-text-field>
        </mdui-card>

        <!-- 插件市场配置 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="font-size:16px;font-weight:500;margin-bottom:16px">插件市场</div>
            <mdui-text-field label="Market API 地址" id="market-api" help-text="插件市场服务端的 API 地址"></mdui-text-field>
        </mdui-card>

        <!-- 内置指令回复模板 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="font-size:16px;font-weight:500;margin-bottom:16px">内置指令回复</div>
            <div style="display:flex;flex-direction:column;gap:16px">
                <mdui-text-field label="/help 回复" id="cmd-help" rows="3" help-text="占位符：{指令列表} {bot名称}"></mdui-text-field>
                <mdui-text-field label="/status 回复" id="cmd-status" rows="3" help-text="占位符：{bot名称} {接收消息数} {发送消息数}"></mdui-text-field>
                <mdui-text-field label="#shizuku 回复" id="cmd-shizuku" rows="4" help-text="占位符：{core版本} {运行时间} {操作系统} {go版本} {bot名称}"></mdui-text-field>
            </div>
        </mdui-card>

        <!-- 机器人配置 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
                <div style="font-size:16px;font-weight:500">机器人列表</div>
                <mdui-button variant="filled" icon="add" id="add-bot-btn">添加机器人</mdui-button>
            </div>
            <div id="bots-config-list"></div>
        </mdui-card>

        <!-- 原始配置 -->
        <mdui-card variant="outlined" class="config-card">
            <div style="font-size:16px;font-weight:500;margin-bottom:16px">config.json (原始)</div>
            <textarea class="config-textarea" id="config-textarea" spellcheck="false" aria-label="原始全局配置"></textarea>
            <p class="plugin-muted">原始配置可保存全局设置，机器人请在机器人页编辑</p>
            <mdui-button id="config-save-raw" variant="outlined">保存原始配置</mdui-button>
        </mdui-card>

        <div class="config-actions">
            <mdui-button variant="filled" icon="save" id="config-save">保存配置</mdui-button>
            <mdui-button variant="outlined" icon="refresh" id="config-reload">重新加载</mdui-button>
        </div>
    `;

    await loadConfig();
    if (epoch !== pageEpoch) return;

    document.getElementById('add-bot-btn').addEventListener('click', addBotConfig);
    document.getElementById('config-save').addEventListener('click', saveConfig);
    document.getElementById('config-save-raw').addEventListener('click',()=>saveConfig(true));
    pageScope.cleanup=()=>configPreferences?.dispose();
    pageScope.dirty = () => Boolean(configBaseline && configDraft() !== configBaseline);
    document.getElementById('config-reload').addEventListener('click', async () => {if (await AdminUI.mayLeave()) loadConfig();});
}

let configData = null;
let configBaseline = '';
let configPreferences = null;
let rawConfigBaseline = '';
function renderConfigPreview() {
    const safe = JSON.parse(JSON.stringify(configData));
    if (safe.webui) safe.webui.password = '';
    for (const bot of safe.bots || []) bot.app_secret = '';
    document.getElementById('config-textarea').value = JSON.stringify(safe, null, 2);
    rawConfigBaseline=document.getElementById('config-textarea').value;
}
function configDraft() {
    const root = currentRoute === '/config' ? pageScope?.container : null;
    if (!root) return '';
    return JSON.stringify([...root.querySelectorAll('mdui-text-field, mdui-select, mdui-switch, textarea')].map(node => [node.id || node.className, node.value, node.checked]));
}

async function loadConfig() {
    try {
        const data = await apiGet('/api/config');
        if (data.code === 0 && data.data) {
            configData = data.data;

            // 填充 WebUI 配置
            const webui = configData.webui || {};
            document.getElementById('webui-port').value = webui.port || 4545;
            document.getElementById('webui-enabled').checked = webui.enabled !== false;
            document.getElementById('webui-password').value = '';

            // 填充插件市场配置
            const marketApiInput = document.getElementById('market-api');
            if (marketApiInput) {
                marketApiInput.value = configData.market_api || 'http://localhost:8081/api';
            }

            // 填充内置指令回复模板
            const builtin = configData.builtin_commands || {};
            const cmdHelp = document.getElementById('cmd-help');
            const cmdStatus = document.getElementById('cmd-status');
            const cmdShizuku = document.getElementById('cmd-shizuku');
            if (cmdHelp) cmdHelp.value = builtin.help || '';
            if (cmdStatus) cmdStatus.value = builtin.status || '';
            if (cmdShizuku) cmdShizuku.value = builtin.shizuku || '';

            // 渲染机器人配置列表
            renderBotsConfigList();

            // Settings preview belongs to this configuration page scope.
            configPreferences?.dispose();
            configPreferences=Preferences.mount(document.getElementById('preferences-settings'),configData,pageScope);
            // 填充原始 JSON
            renderConfigPreview();
            configBaseline = configDraft();
            AdminUI.notice(document.getElementById('config-feedback'), '');
        }
    } catch (e) {
        if (e.name === 'AbortError') return;
        const host = document.getElementById('config-feedback'); if (host) AdminUI.notice(host, '配置加载失败：' + e.message, 'error');
    }
}

function renderBotsConfigList() {
    const listEl = document.getElementById('bots-config-list');
    const bots = configData.bots || [];

    if (bots.length === 0) {
        listEl.innerHTML = '<div style="padding:16px;color:rgb(var(--mdui-color-on-surface-variant))">暂无机器人配置</div>';
        return;
    }

    listEl.innerHTML = bots.map((bot, index) => `
        <div class="bot-config-item" data-index="${index}">
            <mdui-divider style="margin-bottom:12px"></mdui-divider>
            <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px;margin-bottom:12px">
                <mdui-text-field label="ID" value="${escapeHtml(bot.id)}" class="bot-config-id" readonly></mdui-text-field>
                <mdui-text-field label="名称" value="${escapeHtml(bot.name)}" class="bot-config-name"></mdui-text-field>
                <mdui-text-field label="AppID" value="${escapeHtml(bot.app_id)}" class="bot-config-appid"></mdui-text-field>
                <mdui-text-field label="AppSecret" value="" type="password" toggle-password help-text="留空保留原值" class="bot-config-appsecret"></mdui-text-field>
                <div style="display:flex;flex-direction:column;gap:8px">
                    <label style="font-size:14px;color:rgb(var(--mdui-color-on-surface-variant))">连接模式</label>
                    <mdui-select value="${bot.mode || 'webhook'}" class="bot-config-mode">
                        <mdui-menu-item value="websocket">WebSocket</mdui-menu-item>
                        <mdui-menu-item value="webhook">Webhook</mdui-menu-item>
                    </mdui-select>
                </div>
            </div>
            <div style="display:flex;justify-content:flex-end;gap:8px">
                <mdui-button variant="outlined" icon="delete" class="delete-bot-btn" data-index="${index}">删除</mdui-button>
            </div>
        </div>
    `).join('');

    // 绑定删除按钮事件
    listEl.querySelectorAll('.delete-bot-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const index = parseInt(btn.dataset.index);
            configData.bots.splice(index, 1);
            renderBotsConfigList();
        });
    });
}

function addBotConfig() {
    if (!configData.bots) {
        configData.bots = [];
    }
    const newId = 'bot' + Date.now();
    configData.bots.push({
        id: newId,
        name: '',
        app_id: '',
        app_secret: '',
        enabled: false,
        mode: 'webhook'
    });
    renderBotsConfigList();
}

async function saveConfig(raw = false) {
    raw = raw === true;
    const scope = pageScope;
    if (!configData || scope.busy) return;
    const root = scope.container;
    const port = document.getElementById('webui-port');
    const portError = document.getElementById('config-port-error');
    const value = Number(port.value);
    port.error = !raw && (!Number.isInteger(value) || value < 1 || value > 65535);
    portError.textContent = port.error ? '端口须为 1–65535 的整数' : '';
    if (port.error) {port.focus();return;}
    scope.busy = true;
    AdminUI.controlsBusy(root, true);
    try {
        if(raw){
            const parsed=JSON.parse(document.getElementById('config-textarea').value);
            parsed.revision=configData.revision;
            const result=await apiPut('/api/config',parsed);
            configData=result.data;configPreferences.commit(configData.webui.appearance);await loadConfig();showSnackbar('已保存');return;
        }
        if(document.getElementById('config-textarea').value!==rawConfigBaseline)throw Error('原始 JSON 有修改，请保存原始配置或重新加载');
        const settings=configPreferences.read();
        configData.statistics=settings.statistics;
        configData.webui.appearance=settings.appearance;
        // 更新 configData
        configData.webui = {
            ...configData.webui,
            port: parseInt(document.getElementById('webui-port').value) || 4545,
            enabled: document.getElementById('webui-enabled').checked,
            password: document.getElementById('webui-password').value
        };

        // 更新插件市场配置
        const marketApiInput = document.getElementById('market-api');
        if (marketApiInput) {
            configData.market_api = marketApiInput.value.trim();
        }

        // 更新内置指令回复模板
        const cmdHelp = document.getElementById('cmd-help');
        const cmdStatus = document.getElementById('cmd-status');
        const cmdShizuku = document.getElementById('cmd-shizuku');
        configData.builtin_commands = {
            help: cmdHelp ? cmdHelp.value : '',
            status: cmdStatus ? cmdStatus.value : '',
            shizuku: cmdShizuku ? cmdShizuku.value : ''
        };

        // 更新机器人配置
        const botItems = document.querySelectorAll('.bot-config-item');
        botItems.forEach(item => {
            const index = parseInt(item.dataset.index);
            configData.bots[index] = {
                ...configData.bots[index],
                id: item.querySelector('.bot-config-id').value,
                name: item.querySelector('.bot-config-name').value,
                app_id: item.querySelector('.bot-config-appid').value,
                app_secret: item.querySelector('.bot-config-appsecret').value,
                mode: item.querySelector('.bot-config-mode').value
            };
        });

        // 更新原始 JSON
        renderConfigPreview();

        // 保存到服务器
        const data = await apiPut('/api/config', configData);
        if (data.code === 0) {
            configData=data.data;configPreferences.commit(configData.webui.appearance);
            configBaseline = configDraft();
            document.getElementById('webui-password').value = '';
            document.querySelectorAll('.bot-config-appsecret').forEach(node => {node.value = '';});
            configData.webui.password = '';
            for (const bot of configData.bots || []) bot.app_secret = '';
            renderConfigPreview(); configBaseline = configDraft();
            showSnackbar('已保存');
        } else {
            showSnackbar('保存失败: ' + data.message);
        }
    } catch (e) {
        if (e.name === 'AbortError') return;
        AdminUI.notice(document.getElementById('config-feedback'), '保存失败：' + e.message, 'error');
    } finally {scope.busy = false; if (scope.current()) AdminUI.controlsBusy(root, false);}
}

// ==================== 关于页 ====================
// ==================== 用户管理页 ====================
async function renderUsers(container) {
    const epoch = pageEpoch;
    container.innerHTML = `
        <div class="page-title">用户管理</div>
        <mdui-card variant="outlined" class="config-card">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
                <div style="font-size:16px;font-weight:500">用户列表</div>
                <mdui-button variant="filled" icon="person_add" id="add-user-btn">添加用户</mdui-button>
            </div>
            <div id="users-list">加载中...</div>
        </mdui-card>
    `;

    await loadUsers();
    if (epoch !== pageEpoch) return;
    document.getElementById('add-user-btn').addEventListener('click', showAddUserDialog);
}

let usersList = [];

async function loadUsers() {
    try {
        const data = await apiGet('/api/users');
        if (data.code === 0) {
            usersList = data.data || [];
            renderUsersList();
        } else {
            document.getElementById('users-list').innerHTML = 
                '<div style="padding:16px;color:rgb(var(--mdui-color-error))">' + escapeHtml(data.message) + '</div>';
        }
    } catch (e) {
        if (e.name === 'AbortError') return;
        document.getElementById('users-list').innerHTML = 
            '<div style="padding:16px;color:rgb(var(--mdui-color-error))">加载失败: ' + escapeHtml(e.message) + '</div>';
    }
}

function renderUsersList() {
    const listEl = document.getElementById('users-list');
    if (usersList.length === 0) {
        listEl.innerHTML = '<div style="padding:16px;color:rgb(var(--mdui-color-on-surface-variant))">暂无用户</div>';
        return;
    }

    listEl.innerHTML = usersList.map(user => `
        <div style="display:flex;align-items:center;justify-content:space-between;padding:12px 0;border-bottom:1px solid rgb(var(--mdui-color-surface-variant))">
            <div style="display:flex;align-items:center;gap:12px">
                <mdui-icon name="person" style="font-size:24px;color:rgb(var(--mdui-color-primary))"></mdui-icon>
                <div>
                    <div style="font-weight:500">${escapeHtml(user.username)}</div>
                    <div style="font-size:12px;color:rgb(var(--mdui-color-on-surface-variant))">
                        ${user.is_admin ? '管理员' : '普通用户'}
                    </div>
                </div>
            </div>
            <div style="display:flex;gap:8px">
                <mdui-button variant="outlined" size="small" icon="key" onclick="showChangePasswordDialog('${escapeHtml(user.username)}')">修改密码</mdui-button>
                ${user.username !== 'admin' ? 
                    `<mdui-button variant="outlined" size="small" icon="delete" onclick="deleteUser('${escapeHtml(user.username)}')">删除</mdui-button>` : ''}
            </div>
        </div>
    `).join('');
}

function showAddUserDialog() {
    mdui.dialog({closeOnEsc: true,
        headline: '添加用户',
        body: `
            <div style="display:flex;flex-direction:column;gap:16px;padding:8px 0">
                <mdui-text-field label="用户名" id="new-username"></mdui-text-field>
                <mdui-text-field label="密码" type="password" toggle-password id="new-password"></mdui-text-field>
                <mdui-switch label="管理员权限" id="new-is-admin"></mdui-switch>
            </div>
        `,
        actions: [
            { text: '取消', variant: 'text' },
            { 
                text: '确定', 
                variant: 'filled',
                onClick: async (dialog) => {
                    const username = dialog.querySelector('#new-username').value;
                    const password = dialog.querySelector('#new-password').value;
                    const isAdmin = dialog.querySelector('#new-is-admin').checked;

                    if (!username || !password) {
                        showSnackbar('用户名和密码不能为空');
                        return false;
                    }

                    try {
                        const data = await apiPost('/api/users', {
                            username: username,
                            password: password,
                            is_admin: isAdmin
                        });
                        if (data.code === 0) {
                            showSnackbar('添加成功');
                            loadUsers();
                        } else {
                            showSnackbar(data.message || '添加失败');
                            return false;
                        }
                    } catch (e) {
        if (e.name === 'AbortError') return;
                        showSnackbar('添加失败: ' + e.message);
                        return false;
                    }
                }
            }
        ]
    });
}

function showChangePasswordDialog(username) {
    mdui.dialog({closeOnEsc: true,
        headline: '修改密码 - ' + username,
        body: `
            <div style="padding:8px 0">
                <mdui-text-field label="新密码" type="password" toggle-password id="change-password"></mdui-text-field>
            </div>
        `,
        actions: [
            { text: '取消', variant: 'text' },
            { 
                text: '确定', 
                variant: 'tonal',
                onClick: async (dialog) => {
                    const password = dialog.querySelector('#change-password').value;
                    if (!password) {
                        showSnackbar('密码不能为空');
                        return false;
                    }
                    try {
                        const data = await apiPut('/api/users/' + encodeURIComponent(username) + '/password', {
                            password: password
                        });
                        if (data.code === 0) {
                            showSnackbar('修改成功');
                        } else {
                            showSnackbar(data.message || '修改失败');
                            return false;
                        }
                    } catch (e) {
        if (e.name === 'AbortError') return;
                        showSnackbar('修改失败: ' + e.message);
                        return false;
                    }
                }
            }
        ]
    });
}

async function deleteUser(username) {
    try {
        await mdui.confirm({closeOnEsc: true,
            headline: '删除用户',
            description: '确定要删除用户：' + username + ' 吗？此操作不可恢复',
            confirmText: '删除',
            cancelText: '取消'
        });
    } catch {
        return;
    }

    try {
        const data = await apiDelete('/api/users/' + encodeURIComponent(username));
        if (data.code === 0) {
            showSnackbar('删除成功');
            loadUsers();
        } else {
            showSnackbar(data.message || '删除失败');
        }
    } catch (e) {
        if (e.name === 'AbortError') return;
        showSnackbar('删除失败: ' + e.message);
    }
}

async function renderAbout(container) {
    const scope=pageScope;
    const U=AdminUI;
    const info=U.el('dl',{class:'about-info',id:'about-runtime'});
    const links=U.el('div',{class:'about-links'});
    for(const [label,url] of [
        ['GitHub','https://github.com/kylin930/ShizukuBot'],
        ['botgo-plus','https://github.com/kylin930/botgo-plus'],
        ['使用文档','https://github.com/kylin930/ShizukuBot/blob/main/README.md'],
        ['插件开发','https://github.com/kylin930/ShizukuBot/blob/main/docs/plugin-development.md'],
        ['Issues','https://github.com/kylin930/ShizukuBot/issues'],
        ['License','https://github.com/kylin930/ShizukuBot/blob/main/LICENSE']]){
        links.append(U.el('mdui-button',{variant:'text',href:url,target:'_blank',rel:'noopener noreferrer',text:label}));
    }
    const card=U.el('mdui-card',{variant:'outlined',class:'about-card'},[
        U.el('div',{class:'about-logo'},[U.el('mdui-icon',{name:'smart_toy'})]),
        U.text('h1','ShizukuBot','about-title'),
        U.text('p','基于 Go 的 QQ 官方机器人框架，提供多 Bot 管理、Go 插件、规则和 WebUI','about-description'),
        U.text('h2','版本与运行信息'),info,links,
        U.text('h2','技术栈'),U.text('p','Go · botgo-plus · MDUI 2 · SQLite · Chart.js'),
        U.text('h2','开源与许可'),U.text('p','ShizukuBot 使用 Apache License 2.0。感谢上述开源项目及其贡献者。','about-description')]);
    container.replaceChildren(U.text('div','关于','page-title'),U.el('div',{class:'about-container'},[card]));
    try {const result=await scope.request('/api/system');if(!scope.current())return;
        for(const [label,value] of [['Core',result.core_version],['WebUI',result.webui_version],['Go',result.go_version],['系统',result.platform],['运行时间',result.uptime],['SDK',result.sdk+' '+result.sdk_version]])info.append(U.text('dt',label),U.text('dd',value));
    }catch(error){if(scope.current())info.append(U.text('dd','运行信息加载失败'));}
}

// ==================== 机器人管理页 ====================
let allPluginsCache = [];

async function loadAllPlugins() {
    try {
        const data = await apiGet('/api/plugins');
        allPluginsCache = (data.code === 0 ? data.data : data) || [];
    } catch (e) {
        if (e.name === 'AbortError') return;
        allPluginsCache = [];
    }
    return allPluginsCache;
}

async function renderBots(container) {
    const epoch = pageEpoch;
    container.innerHTML = `
        <div class="page-title">
            机器人管理            <mdui-button variant="filled" icon="add" id="add-bot-btn" style="float:right">添加机器人</mdui-button>
        </div>
        <div id="bots-manage-list" style="display:flex;flex-direction:column;gap:16px">
            <div style="text-align:center;color:rgb(var(--mdui-color-on-surface-variant));padding:40px 0">加载中...</div>
        </div>
    `;

    document.getElementById('add-bot-btn')?.addEventListener('click', () => showBotDialog(null, container));

    await loadAllPlugins();
    if (epoch !== pageEpoch) return;
    await loadBotsManageList(container);
}

async function loadBotsManageList(container) {
    const listEl = document.getElementById('bots-manage-list');
    if (!listEl) return;

    try {
        const data = await apiGet('/api/bots');
        const bots = (data.code === 0 ? data.data : data) || [];

        if (bots.length === 0) {
            listEl.innerHTML = '<div style="text-align:center;color:rgb(var(--mdui-color-on-surface-variant));padding:40px 0">暂无机器人，点击右上角添加</div>';
            return;
        }

        const expanded = new Map([...listEl.querySelectorAll('.bot-card')].map(card => [card.dataset.botId, [...card.querySelectorAll('details')].map(details => details.open)]));
        listEl.innerHTML = bots.map(bot => renderBotCard(bot)).join('');
        for (const card of listEl.querySelectorAll('.bot-card')) {
            const opened = expanded.get(card.dataset.botId) || [];
            [...card.querySelectorAll('details')].forEach((details, index) => {details.open = Boolean(opened[index]);});
        }

        // 绑定按钮事件
        listEl.querySelectorAll('[data-action]').forEach(btn => {
            btn.addEventListener('click', async () => {
                const id = btn.dataset.id;
                const action = btn.dataset.action;
                if (btn.loading) return;
                btn.loading = true; btn.disabled = true;
                try {
                    if (action === 'start') {
                        await apiPost(`/api/bots/${id}/start`, {});
                        showSnackbar('已启动');
                        await loadBotsManageList(container);
                    } else if (action === 'stop') {
                        await apiPost(`/api/bots/${id}/stop`, {});
                        showSnackbar('已停止');
                        await loadBotsManageList(container);
                    } else if (action === 'edit') {
                        showBotDialog(id, container);
                    } else if (action === 'plugins') {
                        showBotPluginsDialog(id, container);
                    } else if (action === 'config') {
                        showBotConfigDialog(id, container);
                    } else if (action === 'delete') {
                        try {
                            await mdui.confirm({closeOnEsc: true,
                                headline: '删除机器人',
                                description: '确定删除机器人：' + id + ' 吗？此操作不可恢复',
                                confirmText: '删除',
                                cancelText: '取消'
                            });
                            await apiDelete(`/api/bots/${id}`);
                            await loadBotsManageList(container);
                        } catch {
                            // 用户取消
                        }
                    }
                } catch (e) {
        if (e.name === 'AbortError') return;
                    showSnackbar('操作失败: ' + e.message);
                }
                btn.loading = false; btn.disabled = false;
            });
        });
    } catch (e) {
        if (e.name === 'AbortError') return;
        listEl.innerHTML = '<div style="text-align:center;color:rgb(var(--mdui-color-error));padding:40px 0">加载失败: ' + escapeHtml(e.message) + '</div>';
    }
}

function renderQQDiagnostics(bot) {
    const d = bot.diagnostics || {};
    const time = value => value ? escapeHtml(new Date(value).toLocaleString()) : '暂无记录';
    const categories = {
        authentication: '认证失败', platform_limit: '平台限频',
        missing_reply_id: '缺少消息关联 ID', reply_limit: '本条消息回复次数已达上限',
        passive_reply_expired: '被动回复已过期', passive_reply_time_or_count_limit: '被动回复时间或次数超限',
        not_group_member: '机器人不在目标群', active_send_not_permitted: '平台未允许主动发送',
        target_or_feature_permission: '缺少目标或消息类型权限',
        permission_or_ip_policy: '权限或 IP 策略拒绝（需平台确认）',
        timeout_unknown_delivery: '发送超时，是否送达未知', canceled: '请求已取消',
        unknown_platform_error: '未知平台错误', network_or_protocol: '网络或协议异常'
    };
    const failure = d.last_send_failure ? (categories[d.last_send_failure] || d.last_send_failure) : '';
    const lastError = failure && (!d.last_transport_error || Date.parse(d.last_send_failure_at || 0) >= Date.parse(d.last_transport_error_at || 0)) ? failure : (d.last_transport_error || failure || '暂无记录');
    return `<details class="qq-diagnostics">
        <summary>QQ 诊断</summary>
        <dl class="qq-summary">
            <dt>接入方式</dt><dd>${escapeHtml(bot.mode || 'webhook')} · ${bot.running ? '运行中' : '已停止'}</dd>
            <dt>最近事件</dt><dd>${time(d.last_event_at)}</dd>
            <dt>最近回复</dt><dd>${time(d.last_reply_at)} <small>平台接受</small></dd>
            <dt>最近错误</dt><dd>${escapeHtml(lastError)}</dd>
        </dl>
        <details class="ui-help"><summary>详细诊断</summary>
            <p>事件类型：${escapeHtml(d.last_event_type || '暂无记录')}；消息 ID：${d.last_event_at ? (d.has_message_id ? '已收到' : '本事件未提供') : '未检测'}</p>
            <p>HTTP：${escapeHtml(String(d.http_status || '未检测'))}；平台码：${escapeHtml(String(d.platform_code || '未检测'))}；Trace ID：${escapeHtml(d.trace_id || '暂无记录')}</p>
            <p>最近发送错误：${escapeHtml(failure || '暂无记录')} · ${time(d.last_send_failure_at)}</p>
            <p>最近回调/连接错误：${escapeHtml(d.last_transport_error || '暂无记录')} · ${time(d.last_transport_error_at)}</p>
        </details>
        <details class="ui-help"><summary>接入帮助</summary>
            <p>回调地址：/webhook/${escapeHtml(bot.id)}。检查 HTTPS、事件权限和测试成员，再发送 /ping。</p>
            <p>运行中不代表已收到回调；平台接受不等于 QQ 客户端送达，需在客户端确认。沙盒结果不计入此处。</p>
            <p>建议使用 Webhook；WebSocket 取决于平台授权。IP 白名单和目标权限需在平台检查。</p>
        </details>
    </details>`;
}

function renderBotCard(bot) {
    const statusColor = bot.running ? '#4caf50' : '#9e9e9e';
    const statusText = bot.running ? '运行中' : '已停止';
    const modeText = bot.mode === 'websocket' ? 'WebSocket' : 'Webhook';
    const modeColor = bot.mode === 'websocket' ? '#1976d2' : '#f57c00';
    const btnText = bot.running ? '停止' : '启动';
    const btnIcon = bot.running ? 'stop' : 'play_arrow';
    const btnVariant = bot.running ? 'outlined' : 'filled';
    const btnAction = bot.running ? 'stop' : 'start';
    const pluginCount = bot.plugins && bot.plugins.length ? bot.plugins.length : '全部';
    const prefix = bot.command_prefix || '/';

    return `
        <mdui-card variant="outlined" class="bot-card" data-bot-id="${escapeHtml(bot.id)}" style="padding:16px">
            <div style="display:flex;justify-content:space-between;align-items:flex-start;flex-wrap:wrap;gap:12px">
                <div class="bot-card-info" style="flex:1;min-width:250px">
                    <div class="bot-card-header" style="display:flex;align-items:center;gap:8px;margin-bottom:8px">
                        <span class="bot-status-dot" style="background:${statusColor}"></span>
                        <span style="font-size:18px;font-weight:500">${escapeHtml(bot.name || bot.id)}</span>
                        <span class="bot-mode" style="color:${modeColor};font-size:13px">${modeText}</span>
                        <span style="font-size:13px;color:rgb(var(--mdui-color-on-surface-variant))">${statusText}</span>
                    </div>
                    <div style="color:rgb(var(--mdui-color-on-surface-variant));font-size:13px;line-height:1.8">
                        <div>ID: ${escapeHtml(bot.id)}</div>
                        <div>AppID: ${escapeHtml(bot.app_id)}</div>
                        <div>指令前缀: <code style="background:rgb(var(--mdui-color-surface-variant));padding:2px 6px;border-radius:4px">${escapeHtml(prefix)}</code></div>
                        <div>装载插件: ${pluginCount}${pluginCount !== '全部' ? ' 个' : ''}</div>
                    </div>
                </div>
                <div style="display:flex;gap:8px;flex-wrap:wrap">
                    <mdui-button variant="${btnVariant}" icon="${btnIcon}" data-action="${btnAction}" data-id="${escapeHtml(bot.id)}">${btnText}</mdui-button>
                    <mdui-button variant="outlined" icon="extension" data-action="plugins" data-id="${escapeHtml(bot.id)}">插件</mdui-button>
                    <mdui-button variant="outlined" icon="tune" data-action="config" data-id="${escapeHtml(bot.id)}">配置</mdui-button>
                    <mdui-button variant="outlined" icon="edit" data-action="edit" data-id="${escapeHtml(bot.id)}">编辑</mdui-button>
                    <mdui-button-icon icon="delete" data-action="delete" data-id="${escapeHtml(bot.id)}" style="color:rgb(var(--mdui-color-error))"></mdui-button-icon>
                </div>
            </div>
            ${renderQQDiagnostics(bot)}
        </mdui-card>
    `;
}

// 添加/编辑机器人对话框
function showBotDialog(botId, container) {
    const isEdit = !!botId;
    let bot = { id: '', name: '', app_id: '', app_secret: '', mode: 'webhook', enabled: true };

    if (isEdit) {
        const data = apiGet('/api/bots/' + botId);
        data.then(d => {
            const b = (d.code === 0 ? d.data : d);
            renderForm(b || bot);
        }).catch(error => {if (error.name !== 'AbortError') showSnackbar('加载失败：' + error.message);});
    } else {
        renderForm(bot);
    }

    function renderForm(b) {
        mdui.dialog({closeOnEsc: true,
            headline: isEdit ? '编辑机器人' : '添加机器人',
            body: `
                <div style="display:flex;flex-direction:column;gap:12px;padding:8px 0">
                    <mdui-text-field label="机器人ID" value="${escapeHtml(b.id)}" ${isEdit ? 'disabled' : ''} id="form-id"></mdui-text-field>
                    <mdui-text-field label="名称" value="${escapeHtml(b.name)}" id="form-name"></mdui-text-field>
                    <mdui-text-field label="AppID" value="${escapeHtml(b.app_id)}" id="form-appid"></mdui-text-field>
                    <mdui-text-field label="AppSecret" value="" id="form-secret" type="password" toggle-password></mdui-text-field>
                    <mdui-select label="连接模式" value="${b.mode || 'webhook'}" id="form-mode">
                        <mdui-menu-item value="websocket">WebSocket</mdui-menu-item>
                        <mdui-menu-item value="webhook">Webhook</mdui-menu-item>
                    </mdui-select>
                    <mdui-switch ${b.enabled ? 'checked' : ''} id="form-enabled">启用</mdui-switch>
                </div>
            `,
            actions: [
                { text: '取消', variant: 'text' },
                { 
                    text: '保存', 
                    variant: 'filled',
                    onClick: async (dialog) => {
                        const payload = {
                            id: dialog.querySelector('#form-id').value,
                            name: dialog.querySelector('#form-name').value,
                            app_id: dialog.querySelector('#form-appid').value,
                            app_secret: dialog.querySelector('#form-secret').value,
                            mode: dialog.querySelector('#form-mode').value,
                            enabled: dialog.querySelector('#form-enabled').checked
                        };
                        if (!payload.id || !payload.app_id || (!isEdit && !payload.app_secret)) {
                            showSnackbar('ID、AppID、AppSecret不能为空');
                            return false;
                        }
                        try {
                            if (isEdit) {
                                await apiPut('/api/bots/' + botId, payload);
                            } else {
                                await apiPost('/api/bots', payload);
                            }
                            await loadBotsManageList(container);
                            showSnackbar(isEdit ? '已更新' : '已添加');
                        } catch (e) {
        if (e.name === 'AbortError') return;
                            showSnackbar('保存失败: ' + e.message);
                            return false;
                        }
                    }
                }
            ]
        });
    }
}

// 机器人插件配置对话框（多选）
function showBotPluginsDialog(botId, container) {
    apiGet('/api/bots/' + botId).then(data => {
        const bot = (data.code === 0 ? data.data : data) || {};
        const selectedIds = new Set(bot.plugins || []);
        const plugins = allPluginsCache;

        mdui.dialog({closeOnEsc: true,
            headline: '配置 ' + escapeHtml(bot.name || botId) + ' 的插件',
            body: `
                <div style="padding:8px 0">
                    <div style="margin-bottom:12px;color:rgb(var(--mdui-color-on-surface-variant));font-size:13px">
                        勾选要装载的插件，不勾选任何插件则装载所有启用的插件
                    </div>
                    <div id="plugin-checkbox-list" style="display:flex;flex-direction:column;gap:4px;max-height:400px;overflow-y:auto">
                        ${plugins.length === 0 ? '<div style="padding:20px;text-align:center;color:rgb(var(--mdui-color-on-surface-variant))">暂无插件</div>' :
                            plugins.map(p => `
                                <mdui-checkbox ${selectedIds.has(p.id) ? 'checked' : ''} value="${escapeHtml(p.id)}" id="plugin-${escapeHtml(p.id)}">
                                    ${escapeHtml(p.name || p.id)}${p.enabled ? '' : ' (未启用)'}
                                </mdui-checkbox>
                            `).join('')}
                    </div>
                </div>
            `,
            actions: [
                { text: '取消', variant: 'text' },
                { 
                    text: '保存', 
                    variant: 'filled',
                    onClick: async (dialog) => {
                        const checked = [];
                        plugins.forEach(p => {
                            const cb = dialog.querySelector('#plugin-' + CSS.escape(p.id));
                            if (cb && cb.checked) checked.push(p.id);
                        });
                        try {
                            await apiPut('/api/bots/' + botId, { plugins: checked });
                            await loadBotsManageList(container);
                            showSnackbar('插件配置已保存');
                        } catch (e) {
        if (e.name === 'AbortError') return;
                            showSnackbar('保存失败: ' + e.message);
                            return false;
                        }
                    }
                }
            ]
        });
    }).catch(error => {if (error.name !== 'AbortError') showSnackbar('加载失败：' + error.message);});
}

// 机器人独立配置对话框
function showBotConfigDialog(botId, container) {
    apiGet('/api/bots/' + botId).then(data => {
        const bot = (data.code === 0 ? data.data : data) || {};

        mdui.dialog({closeOnEsc: true,
            headline: escapeHtml(bot.name || botId) + ' - 配置',
            body: `
                <div style="display:flex;flex-direction:column;gap:12px;padding:8px 0">
                    <mdui-text-field label="指令前缀" value="${escapeHtml(bot.command_prefix || '/')}" id="cfg-prefix" helper="符号 / ! #"></mdui-text-field>
                    <mdui-text-field label="管理员用户ID（逗号分隔）" value="${escapeHtml((bot.admin_users || []).join(','))}" id="cfg-admins" helper="多个ID用英文逗号分隔"></mdui-text-field>
                </div>
            `,
            actions: [
                { text: '取消', variant: 'text' },
                { 
                    text: '保存', 
                    variant: 'filled',
                    onClick: async (dialog) => {
                        const prefix = dialog.querySelector('#cfg-prefix').value || '/';
                        const adminsStr = dialog.querySelector('#cfg-admins').value || '';
                        const admins = adminsStr.split(',').map(s => s.trim()).filter(s => s);
                        try {
                            await apiPut('/api/bots/' + botId, { command_prefix: prefix, admin_users: admins });
                            await loadBotsManageList(container);
                            showSnackbar('配置已保存');
                        } catch (e) {
        if (e.name === 'AbortError') return;
                            showSnackbar('保存失败: ' + e.message);
                            return false;
                        }
                    }
                }
            ]
        });
    }).catch(error => {if (error.name !== 'AbortError') showSnackbar('加载失败：' + error.message);});
}

// ==================== 工具函数 ====================
function escapeHtml(text) {
    return String(text ?? '').replace(/[&<>"']/g, char=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
}

let resultSnackbar = null;
function showSnackbar(message) {
    if (customElements.get('mdui-snackbar')) {
        if (resultSnackbar) {resultSnackbar.open = false; resultSnackbar.remove();}
        const snackbar = document.createElement('mdui-snackbar');
        snackbar.textContent = String(message);
        snackbar.autoCloseDelay = 3500;
        snackbar.closeable = true;
        snackbar.addEventListener('closed', () => {snackbar.remove(); if (resultSnackbar === snackbar) resultSnackbar = null;}, {once: true});
        document.body.append(snackbar); resultSnackbar = snackbar; snackbar.open = true;
    } else {window.alert(String(message));}
}

// ==================== 初始化 ====================
document.addEventListener('DOMContentLoaded', () => {
    // 移动端导航项点击
    document.querySelectorAll('.nav-item').forEach(item => {
        item.addEventListener('click', () => {
            navigateTo(item.dataset.route);
            // 点击后关闭移动端抽屉
            const drawer = document.querySelector('.mobile-nav');
            if (drawer) drawer.open = false;
        });
    });

    // 桌面端 NavigationRail change 事件
    const rail = document.querySelector('.desktop-nav');
    if (rail) {
        rail.addEventListener('change', (e) => {
            const selectedItem = rail.querySelector(`mdui-navigation-rail-item[value="${rail.value}"]`);
            if (selectedItem) {
                navigateTo(selectedItem.dataset.route);
            }
        });
    }

    const navDrawer = document.querySelector('.mobile-nav');
    const menuToggle = document.getElementById('menu-toggle');
    if (menuToggle && navDrawer) {
        menuToggle.addEventListener('click', () => {
            navDrawer.open = !navDrawer.open;
        });
    }

    // 登出
    const logoutBtn = document.getElementById('logout-btn');
    if (logoutBtn) {
        logoutBtn.addEventListener('click', async () => {
            await fetch('/logout', {method:'POST',headers:{'Content-Type':'application/json'}}); window.location.href = '/login';
        });
    }

    window.addEventListener('hashchange', () => {const route = location.hash.slice(1) || '/stats'; if (route !== currentRoute) navigateTo(route);});

    // 根据hash路由初始化
    const hash = window.location.hash.replace('#', '') || '/stats';
    navigateTo(hash);
});
