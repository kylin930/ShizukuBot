// Historical marketplace packages are incompatible with compiled Go plugins.
function renderCommunity(container) {
    container.replaceChildren(
        PluginUI.text('h1', '社区插件', 'page-title'),
        PluginUI.el('mdui-card', {variant: 'outlined', class: 'plugin-detail'}, [
            PluginUI.text('h2', '旧格式市场安装已停用'),
            PluginUI.text('p', '历史市场插件不能直接在当前 Go 插件系统运行。请备份旧数据，并按迁移报告人工处理复杂插件。'),
            PluginUI.text('p', '新插件需要审核 Go package、生成注册表、构建并部署。启停和配置可在插件页管理；当前没有在线安装或源码热加载。'),
            PluginUI.el('a', {href: 'https://github.com/kylin930/ShizukuBot/blob/main/docs/plugin-development.md', target: '_blank', rel: 'noopener noreferrer', text: '查看 Go 插件开发说明'}),
        ])
    );
}
