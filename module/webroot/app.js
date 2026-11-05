'use strict';
(() => {
  const $ = id => document.getElementById(id);
  // Full literal allowlist: no user text is ever interpolated into a command.
  const commands = Object.freeze({
    view: '/data/adb/modules/andromeld_guardian/bin/guardian view',
    tick: '/data/adb/modules/andromeld_guardian/bin/guardian tick',
    observe: '/data/adb/modules/andromeld_guardian/bin/guardian policy observe',
    protect: '/data/adb/modules/andromeld_guardian/bin/guardian policy protect',
    '60': '/data/adb/modules/andromeld_guardian/bin/guardian interval 60',
    '120': '/data/adb/modules/andromeld_guardian/bin/guardian interval 120',
    '300': '/data/adb/modules/andromeld_guardian/bin/guardian interval 300'
  });
  const bridged = !!(window.ksu && typeof window.ksu.exec === 'function');
  let busy = false, timer = null, sequence = 0, snapshot = null;
  const object = value => value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  const text = (id, value) => { $(id).textContent = value; };
  const bool = (value, yes, no) => value === true ? yes : value === false ? no : '未知';
  const scalar = value => typeof value === 'number' && Number.isFinite(value) ? String(value) : '未知';
  const bounded = value => typeof value === 'string' ? value.slice(0, 400) : '未知';
  const labels = Object.freeze({ready:'入口已就绪', access_idle_or_disabled:'入口空闲或已关闭', daemon_absent:'未检测到 daemon', unsupported_app_version:'应用版本尚未适配，已停止修复', daemon_target_unconfirmed:'无法确认 daemon 身份，未尝试恢复', daemon_identity_or_version_mismatch:'进程身份或版本不匹配', admin_unresponsive_no_automatic_kill:'管理接口无响应；不会强杀进程', status_incomplete:'状态证据不完整', package_unconfirmed:'尚未确认应用信息', evidence_unavailable:'证据暂不可用', invalid_config_observation_only:'配置异常，仅观察', state_changed:'状态变化', web_wake_requested:'已请求恢复／续期 Web', mac_wake_requested:'已请求恢复／续期 Mac 访问', daemon_bootstrap_requested:'已请求启动 daemon', recovery_unconfirmed:'恢复结果未确认', request_sent_verification_pending:'请求已发出，以后续状态为准'});
  const describe = value => typeof value === 'string' && Object.prototype.hasOwnProperty.call(labels, value) ? labels[value] : bounded(value);
  const timestamp = value => {
    if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T/.test(value)) return null;
    const date = new Date(value);
    return Number.isFinite(date.getTime()) ? date : null;
  };
  const dateText = value => { const date = timestamp(value); return date ? date.toLocaleString('zh-CN', { hour12: false }) : '时间未知'; };
  const badge = (id, label, tone = '') => { text(id, label); $(id).dataset.tone = tone; };
  function feedback(message) { text('feedback', message); $('feedback').hidden = !message; }
  function controls() {
    const cfg = object(object(snapshot).config);
    const modeKnown = cfg.mode === 'observe' || cfg.mode === 'protect';
    const disabled = !bridged || busy || $('confirm').open;
    $('refresh').disabled = !bridged || busy;
    $('policy').disabled = disabled || !modeKnown;
    $('interval').disabled = disabled;
    $('tick').disabled = disabled || !modeKnown;
    $('policy').setAttribute('aria-busy', String(busy));
  }
  function render(data) {
    const cfg = object(data.config), status = object(data.status);
    const checked = timestamp(status.checked_at);
    const observed = checked && checked.getFullYear() >= 2020;
    const daemon = observed ? object(status.daemon) : {};
    const accessKnown = observed && daemon.responsive === true;
    const web = accessKnown ? object(status.web) : {}, mac = accessKnown ? object(status.mac) : {};
    const alive = daemon.pid > 1 ? daemon.alive : status.missing_confirmed === true ? false : undefined;
    const healthLabels = { healthy: '运行正常', degraded: '需要关注', blocked: '恢复受阻', unknown: '状态未知' };
    const age = observed ? Date.now() - checked.getTime() : null;
    // At least two expected intervals plus grace; a future clock is not fresh evidence.
    const interval = [60, 120, 300].includes(cfg.interval) ? cfg.interval : 60;
    const stale = age === null || age > Math.max(120, interval * 2 + 30) * 1000 || age < -60000;
    badge('health', stale ? '状态未确认' : (healthLabels[status.health] || '状态未知'), stale ? 'warn' : status.health === 'healthy' ? 'good' : ['degraded', 'blocked'].includes(status.health) ? 'warn' : '');
    text('updated', observed ? `状态更新 ${dateText(status.checked_at)}${stale ? ' · 已过期或设备时钟异常' : ''}` : '更新时间未知 · 无法确认状态新鲜度');
    badge('daemon-state', bool(alive, '存活', '未运行'), alive === false ? 'warn' : '');
    text('daemon-pid', scalar(daemon.pid > 1 ? daemon.pid : null)); text('daemon-response', bool(alive === undefined ? undefined : daemon.responsive, '有响应', '无响应')); text('daemon-build', scalar(daemon.build));
    badge('web-state', bool(web.enabled, '已开启', '已关闭'));
    text('web-connected', bool(web.connected, '已连接', '未连接')); text('web-port', scalar(web.port)); text('web-keep', bool(cfg.keep_web, '保持开启', '不自动恢复'));
    badge('mac-state', bool(mac.active, '入口可用', '入口未开启'));
    text('mac-connected', bool(mac.connected, '已连接', '未连接')); text('mac-device', bool(mac.device_connected, '已连接', '未连接')); text('mac-keep', bool(cfg.keep_mac, '已配对入口', '不自动恢复'));
    $('policy').setAttribute('aria-checked', String(cfg.mode === 'protect'));
    text('mode-description', cfg.mode === 'protect' ? '保护模式 · 按策略自动修复' : cfg.mode === 'observe' ? '观察模式 · 只读，不自动修复' : '当前策略未知 · 暂不能切换');
    $('interval').value = [60, 120, 300].includes(cfg.interval) ? String(cfg.interval) : '';
    text('worker', `后台任务：${bool(status.worker_running, '运行中', '未运行')}`);
    text('supported', `兼容性：${bool(status.supported, '支持', '不支持')}`);
    text('reason', `诊断原因：${describe(status.reason)}`);
    text('version', `模块版本 ${bounded(data.version)}`);
    const list = $('events'); list.replaceChildren();
    const events = Array.isArray(data.events) ? data.events.slice(-30).reverse() : null;
    text('event-count', events ? `${events.length} 条${data.events.length > 30 ? '（最近 30 条）' : ''}` : '未知');
    if (!events || !events.length) { const li = document.createElement('li'); li.className = 'muted'; li.textContent = events ? '暂无近期事件' : '事件数据未知'; list.append(li); }
    else events.forEach(raw => {
      const event = object(raw), li = document.createElement('li'), time = document.createElement('time'), kind = document.createElement('div'), detail = document.createElement('div');
      time.textContent = dateText(event.at); kind.textContent = describe(event.kind); detail.textContent = describe(event.detail); detail.className = 'event-detail';
      li.append(time, kind, detail); list.append(li);
    });
    controls();
  }
  function run(action) {
    if (!bridged || busy || !Object.prototype.hasOwnProperty.call(commands, action)) return;
    busy = true; controls(); feedback(action === 'view' ? '' : '正在提交，请稍候…');
    const callbackName = `guardianCallback_${Date.now()}_${++sequence}`;
    let expired = false;
    // Do not release the concurrency gate on timeout: exec may still be running.
    const timeout = setTimeout(() => {
      expired = true;
      feedback('后端响应超时，执行结果未知。为避免重复操作，等待原回调期间已暂停刷新和操作；请勿重复提交。');
    }, 20000);
    window[callbackName] = (exitcode, stdout, _stderr) => {
      clearTimeout(timeout); delete window[callbackName]; busy = false;
      try {
        if (typeof stdout !== 'string' || stdout.length > 1048576) throw new Error('后端响应格式或大小异常。');
        let data;
        try { data = JSON.parse(stdout); } catch (_) { throw new Error('后端未返回有效 JSON。'); }
        if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error('后端响应结构异常。');
        if (data.ok !== true) {
          const code = typeof data.error === 'string' && /^[a-zA-Z0-9_.-]{1,80}$/.test(data.error) ? data.error : 'unknown_error';
          throw new Error(`后端拒绝操作：${code}`);
        }
        if (Number(exitcode) !== 0) throw new Error('执行失败（退出码异常），未显示可能含隐私的诊断输出。');
        snapshot = data; render(data);
        feedback(expired ? '已收到延迟回调，状态已更新。' : action === 'view' ? '' : '操作完成，已展示后端返回的状态。');
      } catch (error) {
        feedback(error.message);
        // Old snapshots are not evidence that a mutation failed or succeeded.
        snapshot = null; render({});
      }
      controls();
    };
    try { window.ksu.exec(commands[action], JSON.stringify({}), callbackName); }
    catch (_) {
      clearTimeout(timeout); delete window[callbackName]; busy = false;
      snapshot = null; render({}); feedback('原生桥调用失败，结果未知；请先核实模块状态。'); controls();
    }
  }
  function stopTimer() { if (timer !== null) clearInterval(timer); timer = null; }
  function schedule() {
    stopTimer();
    if (document.hidden || !bridged) return;
    run('view');
    timer = setInterval(() => {
      if (!document.hidden && !$('confirm').open) { if (snapshot) render(snapshot); run('view'); }
    }, 15000);
  }
  $('refresh').addEventListener('click', () => run('view'));
  $('tick').addEventListener('click', () => run('tick'));
  $('interval').addEventListener('change', event => run(event.target.value));
  $('policy').addEventListener('click', () => {
    if (busy || !bridged) return;
    if (object(object(snapshot).config).mode === 'protect') run('observe');
    else if (object(object(snapshot).config).mode === 'observe') { $('confirm').returnValue = ''; $('confirm').showModal(); controls(); }
  });
  $('confirm').addEventListener('close', () => { controls(); if ($('confirm').returnValue === 'protect') run('protect'); });
  document.addEventListener('visibilitychange', () => document.hidden ? stopTimer() : schedule());
  window.addEventListener('pagehide', stopTimer);
  window.addEventListener('pageshow', schedule);
  $('preview').hidden = bridged;
  render({}); controls(); schedule();
})();
