export type IntrospectedType = {
  examples: any[];
  items?: IntrospectedType[];
  properties?: Record<string, IntrospectedType>;
  type: string;
  name?: string;
  description?: string;
  optional: boolean;
  nullable: boolean;
};

export type Endpoint = {
  method: string;
  name: string;
  description: string;
  allPaths: { path: string; sdkPath: string }[];
  queryId?: string | null;
  bodyId?: string | null;
  outputId: string;
  controllerId: string;
  confidential?: boolean;
};

export type Controller = {
  id: string;
  name: string;
  description: string;
};

export type IntrospectPayload = {
  endpoints: Endpoint[];
  controllers: Controller[];
  types: {
    id: string;
    name: string;
    type: IntrospectedType;
  }[];
};

export type IntrospectVersions = {
  versions: { version: string; displayVersion: string; isCurrent: boolean }[];
};

export type IntrospectionSnapshot = {
  versions: IntrospectVersions;
  endpoints: Record<string, IntrospectPayload>;
};

export let getEndpointVersions = async (url: string, snapshot?: IntrospectionSnapshot) => {
  if (snapshot) return snapshot.versions;

  return (await fetch(new URL('/metorial/introspect/versions', url)).then(res =>
    res.json()
  )) as IntrospectVersions;
};

export let getEndpoints = async (
  url: string,
  version: string,
  snapshot?: IntrospectionSnapshot
) => {
  if (snapshot) {
    let payload = snapshot.endpoints[version];
    if (!payload) throw new Error(`Missing introspection snapshot for ${version}.`);
    return payload;
  }

  return (await fetch(
    new URL(`/metorial/introspect/endpoints?version=${version}`, url)
  ).then(res => res.json())) as IntrospectPayload;
};
