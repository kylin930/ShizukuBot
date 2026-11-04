// Historical marketplace packages are incompatible with compiled Go plugins.
function renderCommunity(container) {
    container.replaceChildren(
        PluginUI.text('h1', '社区插件', 'page-title'),
        PluginUI.el('mdui-card', {variant: 'outlined', class: 'plugin-detail'}, [
            PluginUI.text('p', '旧版插件暂不支持'),
            
            PluginUI.help('使用说明', ['新插件需要编译和部署；配置与启停在插件页管理；旧数据请先备份，再按迁移指南处理']),
            PluginUI.el('a', {href: 'https://github.com/kylin930/ShizukuBot/blob/main/docs/plugin-development.md', target: '_blank', rel: 'noopener noreferrer', text: '查看插件开发文档'}),
        ])
    );
}
