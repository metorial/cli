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

export let getEndpointVersions = async (url: string) =>
  (await fetch(new URL('/metorial/introspect/versions', url)).then(res => res.json())) as {
    versions: { version: string; displayVersion: string; isCurrent: boolean }[];
  };

export let getEndpoints = async (url: string, version: string) => {
  return (await fetch(
    new URL(`/metorial/introspect/endpoints?version=${version}`, url)
  ).then(res => res.json())) as IntrospectPayload;
};
