import fetch from 'isomorphic-unfetch';

export const getTransport = endpoint => async (method, args = []) => {
  const uri = endpoint + '/' + encodeURIComponent(method);
  let lastResponse: any;
  console.log('fetching', uri);
  return fetch(uri, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(args),
  })
    .then(r => {
      // console.log(endpoint, 'response', r);
      return r.text();
    })
    .then(text => {
      lastResponse = text;
      return JSON.parse(text);
    })
    .catch(e => {
      console.error('getTransport', e, lastResponse);
      // e.lastResponse = lastResponse;
      return e;
    });
};

export interface ServiceConstructor<ST> extends Function {
  defaultEndpoint: string;
  prototype: ST;
  new (transport: <T>(method: string, data?: any[]) => Promise<T>): ST;
}

export const getAsyncClient = <T>(clientClass: ServiceConstructor<T>, endpoint?: string) => {
  if (endpoint === undefined) {
    endpoint = clientClass.defaultEndpoint;
  }
  return new clientClass(getTransport(endpoint));
};

export const getPrefixedAsyncClient = <T>(clientClass: ServiceConstructor<T>, prefix: string = ''): T => {
  return new clientClass(getTransport(prefix + clientClass.defaultEndpoint));
};
