FROM debian:trixie-slim

# Install curl and bash required for nvm
RUN apt-get update && apt-get install -y curl bash

# Create and switch to runtime user
RUN groupadd -r node && useradd -r -g node -s /bin/bash -m node
USER node

# Install nvm and set up environment
ENV NVM_VERSION=v0.40.3
ENV NVM_DIR=/home/node/.nvm
RUN curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/${NVM_VERSION}/install.sh | bash
ENV PATH="${NVM_DIR}:${PATH}"
RUN echo 'export NVM_DIR="$HOME/.nvm"' >> ~/.bash_profile && \
    echo '[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"' >> ~/.bash_profile && \
    echo '[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"' >> ~/.bash_profile

# Create /app and /app/node_modules directories as runtime user so volume mount preserves ownership
WORKDIR /app
RUN mkdir -p /app/node_modules

# Set entrypoint to login shell which sources ~/.bash_profile (including nvm.sh)
ENTRYPOINT ["/bin/bash", "--login", "-c"]
