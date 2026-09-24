import './style.css';
import { ScanPlugins, UpdatePlugin, SaveOverride } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime';

const app = document.getElementById('app');

app.innerHTML = `
  <div class="header">
    <div>
      <h1>TotalPlug</h1>
      <div class="status-bar" id="status">Нажмите «Сканировать», чтобы проверить плагины...</div>
    </div>
    <div class="actions">
      <button id="btn-scan">🔍 Сканировать плагины</button>
      <button id="btn-refresh" style="background:#555">🔄 Обновить каталог</button>
    </div>
  </div>
  <div class="table-container">
    <table>
      <thead>
        <tr>
          <th>Тип</th>
          <th>Имя плагина</th>
          <th>Установлена</th>
          <th>Источник</th>
          <th>В каталоге</th>
          <th>Статус</th>
          <th>Действие</th>
        </tr>
      </thead>
      <tbody id="plugins-body">
        <tr><td colspan="7" style="text-align:center; color:#777; padding: 24px;">Плагины не загружены</td></tr>
      </tbody>
    </table>
  </div>

  <dialog id="version-editor">
    <form method="dialog" id="editor-form">
      <h2>Версия плагина</h2>
      <div class="editor-plugin" id="editor-plugin"></div>
      <label>
        Установленная версия
        <input type="text" id="editor-version" placeholder="например, 1.8.6.3" autocomplete="off">
      </label>
      <label>
        Источник обновлений
        <input type="text" id="editor-source" placeholder="github:Owner/Repo" autocomplete="off">
      </label>
      <p class="editor-hint">Поля по умолчанию берутся из авто-определения. Пустые значения сбрасывают ручное переопределение.</p>
      <div class="editor-actions">
        <button type="submit" class="btn-save" id="editor-save">Сохранить</button>
        <button type="button" id="editor-cancel">Отмена</button>
      </div>
    </form>
  </dialog>
`;

const statusEl = document.getElementById('status');
const tbody = document.getElementById('plugins-body');
const btnScan = document.getElementById('btn-scan');
const btnRefresh = document.getElementById('btn-refresh');

const editorDialog = document.getElementById('version-editor');
const editorForm = document.getElementById('editor-form');
const editorPluginEl = document.getElementById('editor-plugin');
const editorVersionEl = document.getElementById('editor-version');
const editorSourceEl = document.getElementById('editor-source');
const editorCancelEl = document.getElementById('editor-cancel');

let currentPlugins = [];
let editingPlugin = null;

function getStatusBadge(status) {
    switch (status) {
        case 'up_to_date':
            return '<span class="badge badge-up">✓ Актуален</span>';
        case 'update_avail':
            return '<span class="badge badge-update">🚀 Обновление!</span>';
        case 'not_found':
            return '<span class="badge badge-err">✗ Отсутствует</span>';
        default:
            return '<span class="badge badge-muted">⚠️ Неизвестно</span>';
    }
}

async function runScan(forceRefresh = false) {
    try {
        btnScan.disabled = true;
        btnRefresh.disabled = true;
        statusEl.innerText = forceRefresh ? "Кэш показан, свежий каталог загружается в фоне..." : "Сканирование wincmd.ini и PE-ресурсов (каталог из кэша)...";

        currentPlugins = await ScanPlugins(forceRefresh);

        if (!currentPlugins || currentPlugins.length === 0) {
            tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; padding:20px;">Плагины не найдены в wincmd.ini</td></tr>`;
            statusEl.innerText = "Сканирование завершено: плагины не обнаружены.";
            return;
        }

        renderTable(currentPlugins);
        statusEl.innerText = `Найдено плагинов: ${currentPlugins.length}`;
    } catch (err) {
        statusEl.innerText = `Ошибка: ${err}`;
        console.error(err);
    } finally {
        btnScan.disabled = false;
        btnRefresh.disabled = false;
    }
}

function renderTable(plugins) {
    tbody.innerHTML = plugins.map((p, idx) => {
        const canUpdate = p.status === 'update_avail' && p.download_url;
        return `
            <tr>
                <td><strong>${p.type}</strong></td>
                <td>${p.name}</td>
                <td><code>${p.installed_version}</code></td>
                <td><small style="color:#aaa">${p.confidence}</small></td>
                <td><code>${p.latest_version || '-'}</code></td>
                <td>${getStatusBadge(p.status)}</td>
                <td>
                    ${canUpdate ? `<button class="btn-update" onclick="handleUpdate(${idx})">Обновить</button>` : ''}
                    <button class="btn-edit" title="Указать версию вручную" onclick="handleEdit(${idx})">✎ версия</button>
                </td>
            </tr>
        `;
    }).join('');
}

window.handleEdit = function(index) {
    const plugin = currentPlugins[index];
    if (!plugin) return;

    editingPlugin = plugin;
    editorPluginEl.textContent = `${plugin.name} (${plugin.type})`;

    const auto = plugin.installed_version || '';
    editorVersionEl.value = /^(unknown|n\/a)/i.test(auto) ? '' : auto;
    editorSourceEl.value = plugin.custom_source || '';

    editorDialog.showModal();
};

async function saveOverrideFromEditor() {
    if (!editingPlugin) return;

    const override = {
        custom_version: editorVersionEl.value.trim(),
        custom_source: editorSourceEl.value.trim(),
    };

    editorSaveGuard(true);
    try {
        await SaveOverride(editingPlugin.id, override);
        editorDialog.close();
        statusEl.innerText = `Переопределение для ${editingPlugin.name} сохранено, пересканирую...`;
        runScan(false);
    } catch (err) {
        statusEl.innerText = `Ошибка сохранения переопределения: ${err}`;
    } finally {
        editorSaveGuard(false);
    }
}

function editorSaveGuard(busy) {
    document.getElementById('editor-save').disabled = busy;
}

editorForm.addEventListener('submit', async (e) => {
    // method="dialog" would close before the async save finishes
    e.preventDefault();
    await saveOverrideFromEditor();
});

editorCancelEl.addEventListener('click', () => editorDialog.close());

window.handleUpdate = async function(index) {
    const plugin = currentPlugins[index];
    if (!plugin) return;

    if (!confirm(`Обновить плагин ${plugin.name} до версии ${plugin.latest_version}?`)) {
        return;
    }

    try {
        statusEl.innerText = `Обновление ${plugin.name}...`;
        const result = await UpdatePlugin(plugin);

        if (result && result.locked) {
            statusEl.innerText = `⚠ Файлы плагина ${plugin.name} заняты. Закройте Total Commander и повторите обновление.`;
            return;
        }

        alert(result && result.backup_path
            ? `Плагин ${plugin.name} обновлен! Резервная копия: ${result.backup_path}`
            : `Плагин ${plugin.name} успешно обновлен!`);
        runScan(false);
    } catch (err) {
        alert(`Ошибка обновления: ${err}`);
        statusEl.innerText = `Ошибка при обновлении ${plugin.name}`;
    }
};

btnScan.addEventListener('click', () => runScan(false));
btnRefresh.addEventListener('click', () => runScan(true));

// Фоновое обновление каталога (запускается Go-мостом после каждого скана)
EventsOn('plugins:refreshed', (plugins) => {
    if (!plugins || plugins.length === 0) return;
    currentPlugins = plugins;
    renderTable(currentPlugins);
    statusEl.innerText = `Данные из кэша, каталог обновлён. Плагинов: ${currentPlugins.length}`;
});

EventsOn('plugins:refresh:failed', (msg) => {
    statusEl.innerText = `⚠ Фоновое обновление каталога не удалось: ${msg}`;
});

// Автозапуск кэш-скана при старте окна (без сетевого ожидания)
runScan(false);