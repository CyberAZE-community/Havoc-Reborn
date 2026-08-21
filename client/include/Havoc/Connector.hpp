#ifndef HAVOC_CONNECTOR_HPP
#define HAVOC_CONNECTOR_HPP

#include <global.hpp>
#include <QJsonDocument>
#include <QJsonObject>
#include <QWebSocket>
#include <QObject>

#include <Havoc/Packager.hpp>

namespace HavocNamespace
{
    class Connector : public QObject
    {
    private:
        QWebSocket*           Socket        = nullptr;
        Util::ConnectionInfo* Teamserver    = nullptr;
        HavocSpace::Packager* Packager      = nullptr;
        bool                  UserDisconnect = false;

    public:
        QString ErrorString = QString();

        Connector( Util::ConnectionInfo* );
        ~Connector() noexcept;

        bool Disconnect();

        void SendLogin();
        void SendPackage( Util::Packager::PPackage package );
    };
}

#endif //HAVOC_CONNECTOR_HPP
