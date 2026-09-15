export let camelToKebab = (value: string) => {
  return value
    .replace(/_/g, '-')
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1-$2')
    .toLowerCase();
};

export let kebabToCamel = (value: string) => {
  return value.replace(/[-_]([a-z0-9])/g, (_, char: string) => char.toUpperCase());
};

export let toKebab = (value: string) => camelToKebab(value);

export let normalizeSdkPath = (sdkPath: string) =>
  sdkPath
    .split('.')
    .map(part => camelToKebab(part))
    .join('.');

export let stripSdkPath = (sdkPath: string) => {
  let path = sdkPath;
  if (path.startsWith('management.instance.')) {
    path = path.slice('management.instance.'.length);
  } else if (path.startsWith('management.organization.')) {
    path = path.slice('management.organization.'.length);
  }
  return path;
};

export let splitResourceMethod = (sdkPath: string) => {
  let parts = sdkPath.split('.');
  let method = parts.pop() || '';
  return {
    resource: parts.join('.'),
    method
  };
};

export let pathPlaceholders = (path: string) =>
  path
    .split('/')
    .filter(part => part.startsWith(':') && part.length > 1)
    .map(part => part.slice(1));
