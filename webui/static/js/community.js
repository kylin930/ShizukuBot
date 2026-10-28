// ShizukuBot 插件社区模块 - 从 OSS 获取插件数据

// ==================== 社区全局状态 ====================
let communityOssBase = '';
let communityPageSize = 50;
let communityVolumeMax = 300;
let communityPluginsCache = null;
let communityCurrentPage = 1;
let communitySearchQuery = '';
let communityCategoryFilter = '';
let communityStatsCache = null;

const INDEX_FILE = 'hpuhtrz0vfl291jrv8ay_index.json';

const CATEGORIES = {
    'utility': '实用工具',
    'entertainment': '娱乐互动',
    'management': '管理辅助',
    'ai': 'AI 智能',
    'social': '社交互动',
    'media': '媒体处理',
    'chat': '聊天',
    'tool': '工具',
    'game': '游戏',
    'admin': '管理',
    'other': '其他'
};

const CATEGORY_ICONS = {
    'utility': 'wrench',
    'entertainment': 'gamepad_2',
    'management': 'manage_accounts',
    'ai': 'brain',
    'social': 'message',
    'media': 'image',
    'chat': 'message',
    'tool': 'build',
    'game': 'gamepad_2',
    'admin': 'shield',
    'other': 'extension'
};

// ==================== 主渲染入口 ====================
async function renderCommunity(container) {
    container.innerHTML = `
        <div class="page-title">插件社区</div>
        <div id="community-stats"></div>
        <div class="community-header">
            <mdui-text-field id="community-search" label="搜索插件" icon="search" clearable></mdui-text-field>
            <mdui-select id="community-category" label="分类" value="">
                <mdui-menu-item value="">全部分类</mdui-menu-item>
                ${Object.entries(CATEGORIES).map(([k, v]) => `<mdui-menu-item value="${k}">${v}</mdui-menu-item>`).join('')}
            </mdui-select>
            <mdui-button variant="filled" icon="upload" id="community-upload-btn">发布插件</mdui-button>
        </div>
        <div id="community-plugins"></div>
        <div id="community-pagination" style="display:flex;justify-content:center;gap:8px;margin-top:24px"></div>
    `;

    document.getElementById('community-search').addEventListener('input', (e) => {
        communitySearchQuery = e.target.value.trim().toLowerCase();
        communityCurrentPage = 1;
        renderCommunityPlugins();
    });

    document.getElementById('community-category').addEventListener('change', (e) => {
        communityCategoryFilter = e.target.value;
        communityCurrentPage = 1;
        renderCommunityPlugins();
    });

    document.getElementById('community-upload-btn').addEventListener('click', showUploadPluginDialog);

    // 并行加载数据和统计
    await loadCommunityData();
    loadCommunityStats();
}

// ==================== 从 OSS 加载插件数据 ====================
async function loadCommunityData() {
    try {
        // 1. 从主服务获取 market_api 地址
        const mainConfig = await fetch('/api/config').then(r => r.json());
        const marketApi = (mainConfig.code === 0 && mainConfig.data && mainConfig.data.market_api)
            ? mainConfig.data.market_api
            : 'http://localhost:8081/api';

        // 2. 从市场服务获取 oss_base 等配置
        const marketConfig = await fetch(marketApi + '/config').then(r => r.json());
        if (marketConfig.code === 0 && marketConfig.data) {
            communityOssBase = marketConfig.data.oss_base;
            communityPageSize = marketConfig.data.page_size || 50;
            communityVolumeMax = marketConfig.data.volume_max || 300;
        }

        if (!communityOssBase) {
            document.getElementById('community-plugins').innerHTML =
                '<mdui-card variant="outlined" style="padding:24px;text-align:center;color:var(--mdui-color-on-surface-variant)">插件市场服务未配置</mdui-card>';
            return;
        }

        // 3. 从 OSS 获取索引文件
        const indexUrl = communityOssBase + '/' + INDEX_FILE;
        const indexResp = await fetch(indexUrl);
        if (!indexResp.ok) {
            if (indexResp.status === 404) {
                communityPluginsCache = [];
                renderCommunityPlugins();
                return;
            }
            throw new Error('获取索引失败: HTTP ' + indexResp.status);
        }

        const index = await indexResp.json();

        // 4. 获取所有卷文件并合并插件
        const allPlugins = [];
        for (const vol of (index.volumes || [])) {
            try {
                const volUrl = communityOssBase + '/' + vol.file;
                const volResp = await fetch(volUrl);
                if (volResp.ok) {
                    const volData = await volResp.json();
                    allPlugins.push(...(volData.plugins || []));
                }
            } catch (e) {
                console.warn('加载卷文件失败:', vol.file, e);
            }
        }

        // 按更新时间降序排列
        allPlugins.sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0));
        communityPluginsCache = allPlugins;

        renderCommunityPlugins();
    } catch (e) {
        console.error('Failed to load community data:', e);
        const pluginsEl = document.getElementById('community-plugins');
        if (pluginsEl) {
            pluginsEl.innerHTML =
                '<mdui-card variant="outlined" style="padding:24px;text-align:center;color:var(--mdui-color-error)">加载失败: ' + escapeHtml(e.message) + '</mdui-card>';
        }
    }
}

// ==================== 加载统计 ====================
async function loadCommunityStats() {
    try {
        const mainConfig = await fetch('/api/config').then(r => r.json());
        const marketApi = (mainConfig.code === 0 && mainConfig.data && mainConfig.data.market_api)
            ? mainConfig.data.market_api
            : 'http://localhost:8081/api';

        const statsResp = await fetch(marketApi + '/stats');
        if (statsResp.ok) {
            const statsData = await statsResp.json();
            if (statsData.code === 0 && statsData.data) {
                communityStatsCache = statsData.data;
            }
        }
    } catch (e) {
        console.warn('Failed to load community stats:', e);
    }

    const statsEl = document.getElementById('community-stats');
    if (!statsEl) return;

    const total = communityStatsCache ? communityStatsCache.total_plugins : (communityPluginsCache ? communityPluginsCache.length : 0);
    const downloads = communityStatsCache ? communityStatsCache.total_downloads : 0;

    statsEl.innerHTML = `
        <div class="community-stats">
            <mdui-card variant="outlined" class="community-stat-card">
                <div class="community-stat-value">${total}</div>
                <div class="community-stat-label">插件总数</div>
            </mdui-card>
            <mdui-card variant="outlined" class="community-stat-card">
                <div class="community-stat-value">${downloads}</div>
                <div class="community-stat-label">总下载量</div>
            </mdui-card>

        </div>
    `;
}

// ==================== 渲染插件列表 ====================
function renderCommunityPlugins() {
    const pluginsEl = document.getElementById('community-plugins');
    const paginationEl = document.getElementById('community-pagination');
    if (!pluginsEl || !communityPluginsCache) return;

    // 过滤
    let filtered = communityPluginsCache;
    if (communitySearchQuery) {
        filtered = filtered.filter(p =>
            (p.name || '').toLowerCase().includes(communitySearchQuery) ||
            (p.description || '').toLowerCase().includes(communitySearchQuery) ||
            (p.author || '').toLowerCase().includes(communitySearchQuery) ||
            (p.keywords || []).some(k => k.toLowerCase().includes(communitySearchQuery))
        );
    }
    if (communityCategoryFilter) {
        filtered = filtered.filter(p => p.category === communityCategoryFilter);
    }

    // 分页
    const totalPages = Math.max(1, Math.ceil(filtered.length / communityPageSize));
    if (communityCurrentPage > totalPages) communityCurrentPage = totalPages;
    const start = (communityCurrentPage - 1) * communityPageSize;
    const pagePlugins = filtered.slice(start, start + communityPageSize);

    if (filtered.length === 0) {
        pluginsEl.innerHTML =
            '<mdui-card variant="outlined" style="padding:24px;text-align:center;color:var(--mdui-color-on-surface-variant)">暂无插件</mdui-card>';
        paginationEl.innerHTML = '';
        return;
    }

    pluginsEl.innerHTML = '<div class="plugins-grid">' + pagePlugins.map(p => renderPluginCard(p)).join('') + '</div>';

    // 绑定安装按钮事件
    pluginsEl.querySelectorAll('.plugin-install-btn').forEach(btn => {
        btn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const pluginId = btn.dataset.pluginId;
            const downloadUrl = btn.dataset.downloadUrl;
            if (!downloadUrl) {
                showSnackbar('下载链接无效');
                return;
            }
            try {
                btn.setAttribute('loading', '');

                // 优化：同时发起两个请求，不等待市场服务端响应
                // 1. 异步发送下载统计请求（不等待响应）
                sendDownloadStats(pluginId);

                // 2. 直接从 OSS 下载插件 JSON 文件
                const resp = await fetch(downloadUrl);
                if (!resp.ok) throw new Error('下载失败: HTTP ' + resp.status);
                const pluginJson = await resp.json();

                // 提交到本地插件系统
                const data = await apiPost('/api/plugins', {
                    id: pluginJson.id || pluginId,
                    name: pluginJson.name || '未命名插件',
                    description: pluginJson.description || '',
                    version: pluginJson.version || '1.0.0',
                    enabled: true,
                    nodes: pluginJson.nodes || [],
                    graph: pluginJson.graph || {}
                });
                if (data.code === 0) {
                    showSnackbar('安装成功');
                } else {
                    showSnackbar(data.message || '安装失败');
                }
            } catch (e) {
                showSnackbar('安装失败: ' + e.message);
            } finally {
                btn.removeAttribute('loading');
            }
        });
    });

    // 分页按钮
    if (totalPages <= 1) {
        paginationEl.innerHTML = '';
        return;
    }

    let paginationHtml = '';
    if (communityCurrentPage > 1) {
        paginationHtml += `<mdui-button variant="outlined" data-page="${communityCurrentPage - 1}">上一页</mdui-button>`;
    }
    paginationHtml += `<span style="display:flex;align-items:center;padding:0 12px;color:var(--mdui-color-on-surface-variant)">${communityCurrentPage} / ${totalPages}</span>`;
    if (communityCurrentPage < totalPages) {
        paginationHtml += `<mdui-button variant="outlined" data-page="${communityCurrentPage + 1}">下一页</mdui-button>`;
    }
    paginationEl.innerHTML = paginationHtml;

    paginationEl.querySelectorAll('mdui-button').forEach(btn => {
        btn.addEventListener('click', () => {
            communityCurrentPage = parseInt(btn.dataset.page);
            renderCommunityPlugins();
            window.scrollTo({ top: 0, behavior: 'smooth' });
        });
    });
}

// ==================== 插件卡片 ====================
function renderPluginCard(p) {
    const categoryName = CATEGORIES[p.category] || p.category || '其他';
    const categoryIcon = CATEGORY_ICONS[p.category] || 'extension';
    const keywords = (p.keywords || []).map(k =>
        `<span class="plugin-card-tag">${escapeHtml(k)}</span>`
    ).join('');
    const updated = p.updated_at ? new Date(p.updated_at * 1000).toLocaleDateString('zh-CN') : '';
    const description = truncateText(p.description || '', 80);
    const downloads = p.downloads || 0;

    return `
        <mdui-card variant="outlined" class="community-plugin-card" clickable>
            <div class="plugin-card-header">
                <div class="plugin-card-icon">
                    <mdui-icon name="${categoryIcon}"></mdui-icon>
                </div>
                <div class="plugin-card-info">
                    <div class="plugin-card-name">${escapeHtml(p.name)}</div>
                    <div class="plugin-card-author">by ${escapeHtml(p.author)}</div>
                </div>
            </div>
            <div class="plugin-card-desc">${escapeHtml(description)}</div>
            <div class="plugin-card-meta">
                <span class="plugin-card-tag">${categoryName}</span>
                ${keywords}
            </div>
            <div class="plugin-card-footer">
                <div class="plugin-card-footer-left">
                    <span class="plugin-card-version">v${escapeHtml(p.version || '1.0.0')}</span>
                    <span class="plugin-card-downloads">⬇ ${downloads}</span>
                    <span class="plugin-card-updated">${updated}</span>
                </div>
                <mdui-button variant="tonal" size="small" data-plugin-id="${escapeHtml(p.id)}" data-download-url="${escapeHtml(p.download_url || '')}" class="plugin-install-btn">安装</mdui-button>
            </div>
        </mdui-card>
    `;
}

// ==================== 文本截断 ====================
function truncateText(text, maxLength) {
    if (!text) return '';
    if (text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
}

// ==================== 发送下载统计（异步，不等待响应） ====================
async function sendDownloadStats(pluginId) {
    try {
        const mainConfig = await fetch('/api/config').then(r => r.json());
        const marketApi = (mainConfig.code === 0 && mainConfig.data && mainConfig.data.market_api)
            ? mainConfig.data.market_api
            : 'http://localhost:8081/api';
        
        fetch(marketApi + '/plugins/' + encodeURIComponent(pluginId) + '/download', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        }).catch(() => {});
    } catch (e) {
        console.warn('Failed to send download stats:', e);
    }
}

// ==================== 分类名称 ====================
function getCategoryName(key) {
    return CATEGORIES[key] || key || '其他';
}

// ==================== 发布插件对话框 ====================
function showUploadPluginDialog() {
    mdui.dialog({
        headline: '发布插件',
        fullscreen: true,
        closeOnEsc: true,
        closeOnOverlayClick: true,
        body: `
            <div style="display:flex;flex-direction:column;gap:16px;padding:8px 0">
                <mdui-text-field id="upload-name" label="插件名称" maxlength="50" required></mdui-text-field>
                <mdui-text-field id="upload-author" label="插件作者" maxlength="50" required></mdui-text-field>
                <mdui-text-field id="upload-desc" label="插件描述" maxlength="500" rows="4" required></mdui-text-field>
                <mdui-text-field id="upload-version" label="版本号" maxlength="20" value="1.0.0" helper="例如: 1.0.0"></mdui-text-field>
                <mdui-text-field id="upload-keywords" label="关键词（逗号分隔）" maxlength="200" helper="例如: 聊天,AI,工具"></mdui-text-field>
                <mdui-select id="upload-category" label="分类" value="other">
                    <mdui-menu-item value="chat">聊天</mdui-menu-item>
                    <mdui-menu-item value="tool">工具</mdui-menu-item>
                    <mdui-menu-item value="game">游戏</mdui-menu-item>
                    <mdui-menu-item value="admin">管理</mdui-menu-item>
                    <mdui-menu-item value="entertainment">娱乐</mdui-menu-item>
                    <mdui-menu-item value="other">其他</mdui-menu-item>
                </mdui-select>
                <mdui-text-field id="upload-password" label="密码（可选，用于后续更新验证）" type="password" toggle-password></mdui-text-field>

                <mdui-card variant="outlined" style="padding:16px;display:flex;flex-direction:column;gap:12px">
                    <div style="font-size:14px;color:var(--mdui-color-on-surface-variant)">上传插件 JSON 文件</div>
                    <input type="file" id="upload-file" accept=".json,application/json" style="display:none">
                    <mdui-button variant="outlined" id="upload-select-btn" icon="attach_file">选择文件</mdui-button>
                    <div id="upload-file-name" style="font-size:14px;color:var(--mdui-color-on-surface-variant)">未选择文件</div>
                    <mdui-text-field id="upload-file-content" label="或直接粘贴 JSON 内容" rows="6"></mdui-text-field>
                </mdui-card>
            </div>
        `,
        actions: [
            { text: '取消', variant: 'text' },
            { 
                text: '发布', 
                variant: 'filled',
                icon: 'upload',
                onClick: async (dialog) => {
                    const name = dialog.querySelector('#upload-name')?.value?.trim() || '';
                    const author = dialog.querySelector('#upload-author')?.value?.trim() || '';
                    const description = dialog.querySelector('#upload-desc')?.value?.trim() || '';
                    const version = dialog.querySelector('#upload-version')?.value?.trim() || '1.0.0';
                    const keywordsStr = dialog.querySelector('#upload-keywords')?.value?.trim() || '';
                    const category = dialog.querySelector('#upload-category')?.value || 'other';
                    const password = dialog.querySelector('#upload-password')?.value || '';
                    const fileContentText = dialog.querySelector('#upload-file-content')?.value?.trim() || '';
                    const fileInput = dialog.querySelector('#upload-file');
                    const selectedFile = fileInput.files[0] || null;

                    if (!name || !author || !description) {
                        showSnackbar('插件名称、作者、描述为必填项');
                        return false;
                    }

                    // 解析关键词
                    const keywords = keywordsStr.split(/[,，]/).map(s => s.trim()).filter(s => s);

                    let pluginJson;
                    let fileBlob;

                    try {
                        if (selectedFile) {
                            const text = await selectedFile.text();
                            pluginJson = JSON.parse(text);
                            fileBlob = new Blob([text], { type: 'application/json' });
                        } else if (fileContentText) {
                            pluginJson = JSON.parse(fileContentText);
                            fileBlob = new Blob([fileContentText], { type: 'application/json' });
                        } else {
                            showSnackbar('请选择 JSON 文件或粘贴 JSON 内容');
                            return false;
                        }
                    } catch (e) {
                        showSnackbar('JSON 格式错误: ' + e.message);
                        return false;
                    }

                    const submitBtn = dialog.querySelector('mdui-button[slot="action"]:last-child');
                    submitBtn.setAttribute('loading', '');

                    try {
                        const { marketApi, sts } = await getMarketSTS();

                        const filename = generateHexUUID() + '.json';

                        const client = new OSS({
                            region: sts.region,
                            accessKeyId: sts.AccessKeyId,
                            accessKeySecret: sts.AccessKeySecret,
                            stsToken: sts.SecurityToken,
                            bucket: sts.bucket
                        });

                        await client.put(filename, fileBlob);

                        const downloadURL = sts.public_base + '/' + filename;

                        const data = await fetch(marketApi + '/plugins', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({
                                name: name,
                                author: author,
                                description: description,
                                keywords: keywords,
                                category: category,
                                version: version,
                                download_url: downloadURL,
                                password: password
                            })
                        });

                        const result = await data.json();
                        if (result.code === 0) {
                            showSnackbar('发布成功');
                            await loadCommunityData();
                        } else {
                            showSnackbar(result.message || '发布失败');
                            return false;
                        }
                    } catch (e) {
                        showSnackbar('发布失败: ' + e.message);
                        return false;
                    } finally {
                        submitBtn.removeAttribute('loading');
                    }
                }
            }
        ],
        onOpen: (dialog) => {
            let selectedFile = null;
            const fileInput = dialog.querySelector('#upload-file');
            const selectBtn = dialog.querySelector('#upload-select-btn');
            const fileNameEl = dialog.querySelector('#upload-file-name');

            selectBtn.addEventListener('click', () => fileInput.click());

            fileInput.addEventListener('change', () => {
                selectedFile = fileInput.files[0] || null;
                if (selectedFile) {
                    fileNameEl.textContent = selectedFile.name;
                    fileNameEl.style.color = 'var(--mdui-color-on-surface)';
                } else {
                    fileNameEl.textContent = '未选择文件';
                    fileNameEl.style.color = 'var(--mdui-color-on-surface-variant)';
                }
            });
        }
    });
}

// ==================== 获取市场服务 STS ====================
async function getMarketSTS() {
    const mainConfig = await fetch('/api/config').then(r => r.json());
    const marketApi = (mainConfig.code === 0 && mainConfig.data && mainConfig.data.market_api)
        ? mainConfig.data.market_api
        : 'http://localhost:8081/api';

    const stsResp = await fetch(marketApi + '/sts');
    const stsData = await stsResp.json();
    if (stsData.code !== 0) {
        throw new Error(stsData.message || '获取STS凭证失败');
    }

    return { marketApi, sts: stsData.data };
}

// ==================== 32 位随机 hex UUID ====================
function generateHexUUID() {
    const chars = '0123456789abcdef';
    let result = '';
    for (let i = 0; i < 32; i++) {
        result += chars[Math.floor(Math.random() * 16)];
    }
    return result;
}

// ==================== 从插件 JSON 提取关键词 ====================
function extractKeywords(pluginJson) {
    const keywords = [];
    if (pluginJson.name) {
        keywords.push(...String(pluginJson.name).split(/[\s_-]+/).filter(s => s.length >= 2));
    }
    if (pluginJson.category && !keywords.includes(pluginJson.category)) {
        keywords.push(pluginJson.category);
    }
    if (pluginJson.author && !keywords.includes(pluginJson.author)) {
        keywords.push(pluginJson.author);
    }
    return keywords.slice(0, 10);
}

// ==================== OSS 客户端签名上传（兼容旧接口） ====================
async function ossSign(uploadUrl, fileData, contentType) {
    try {
        const { sts } = await getMarketSTS();

        const client = new OSS({
            region: sts.region,
            accessKeyId: sts.AccessKeyId,
            accessKeySecret: sts.AccessKeySecret,
            stsToken: sts.SecurityToken,
            bucket: sts.bucket_name,
            endpoint: sts.endpoint,
            secure: true,
            cname: false
        });

        // uploadUrl 在这里应为 object key
        const result = await client.put(uploadUrl, fileData, {
            headers: { 'Content-Type': contentType || 'application/json' }
        });

        return result.url;
    } catch (e) {
        console.error('OSS upload error:', e);
        throw e;
    }
}
