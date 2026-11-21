# One image per server: docker build --build-arg SERVER=SkillServer -t skillserver dotnet/
FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build
ARG SERVER
WORKDIR /src
COPY Directory.Build.props ./
COPY src/ src/
RUN test -n "$SERVER" && dotnet publish src/$SERVER -c Release -o /out /p:UseAppHost=false \
    && ln -s $SERVER.dll /out/Server.dll && ln -s $SERVER.runtimeconfig.json /out/Server.runtimeconfig.json && ln -s $SERVER.deps.json /out/Server.deps.json

FROM mcr.microsoft.com/dotnet/aspnet:8.0-noble-chiseled
WORKDIR /app
COPY --from=build /out ./
ENV ASPNETCORE_URLS= DOTNET_EnableDiagnostics=0
EXPOSE 8080
USER app
ENTRYPOINT ["dotnet", "Server.dll"]
