import { externalizeNuxtBootstrap } from '../utils/cspExternalize';

export default defineNitroPlugin((nitro) => {
  nitro.hooks.hook('render:response', (response, context) => {
    if (typeof response.body !== 'string') return;
    const runtimeConfig = useRuntimeConfig(context.event);
    response.body = externalizeNuxtBootstrap(response.body, {
      app: runtimeConfig.app,
      public: runtimeConfig.public,
    });
  });
});
